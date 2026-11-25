package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/jsonb"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
)

// ================================================================== trip plan

// safeURL accepts only http(s) links (blocks javascript:, data:, ...).
func safeURL(field string, v *string) (*string, error) {
	v = trim(v)
	if v == nil {
		return nil, nil
	}
	u, err := url.Parse(*v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, validation.Field(field, "url", "must be an http(s) link, e.g. a Google Maps share link")
	}
	return v, nil
}

func toPlan(p *domain.TripPlan, bookingID uuid.UUID, canEdit bool) dto.TripPlanResponse {
	r := dto.TripPlanResponse{BookingID: bookingID.String(), Itinerary: []dto.ItineraryItem{}, CanEdit: canEdit}
	if p == nil {
		return r
	}
	r.Exists = true
	r.ArrivalAt, r.ArrivalFrom, r.ArrivalMode = p.ArrivalAt, p.ArrivalFrom, p.ArrivalMode
	r.PickupName, r.PickupMapURL, r.HotelName, r.HotelMapURL = p.PickupName, p.PickupMapURL, p.HotelName, p.HotelMapURL
	r.Notes, r.UpdatedAt = p.Notes, &p.UpdatedAt
	if p.HotelCheckIn != nil {
		v := p.HotelCheckIn.Format(dateFmt)
		r.HotelCheckIn = &v
	}
	if p.HotelCheckOut != nil {
		v := p.HotelCheckOut.Format(dateFmt)
		r.HotelCheckOut = &v
	}
	var items []domain.ItineraryItem
	_ = p.Itinerary.Decode(&items)
	for _, it := range items {
		r.Itinerary = append(r.Itinerary, dto.ItineraryItem{Date: it.Date, Time: it.Time, Title: it.Title, Place: it.Place,
			MapURL: it.MapURL, Note: it.Note})
	}
	if p.UpdatedBy != nil {
		r.UpdatedBy = person(p.Editor, *p.UpdatedBy)
	}
	return r
}

// GetPlan returns the trip plan of a booking (tourist, guide, admin).
func (s *Service) GetPlan(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID) (*dto.TripPlanResponse, error) {
	b, err := s.load(ctx, viewer, role, bookingID)
	if err != nil {
		return nil, err
	}
	canEdit := b.IsParty(viewer) && b.Status == domain.StatusAccepted && b.IsPaid()
	p, err := s.repo.GetPlan(ctx, bookingID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			r := toPlan(nil, bookingID, canEdit)
			return &r, nil
		}
		return nil, err
	}
	r := toPlan(p, bookingID, canEdit)
	return &r, nil
}

// SavePlan replaces the trip plan (tourist or guide, once paid and before completion).
func (s *Service) SavePlan(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID, req dto.TripPlanRequest) (*dto.TripPlanResponse, error) {
	b, err := s.load(ctx, viewer, role, bookingID)
	if err != nil {
		return nil, err
	}
	if !b.IsParty(viewer) {
		return nil, domain.ErrReadOnly
	}
	if !(b.Status == domain.StatusAccepted && b.IsPaid()) {
		return nil, domain.ErrPlanClosed
	}

	p := &domain.TripPlan{BookingID: bookingID, ArrivalAt: req.ArrivalAt, ArrivalFrom: trim(req.ArrivalFrom),
		ArrivalMode: trim(req.ArrivalMode), PickupName: trim(req.PickupName), HotelName: trim(req.HotelName),
		Notes: trim(req.Notes), UpdatedBy: &viewer, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if p.PickupMapURL, err = safeURL("pickup_map_url", req.PickupMapURL); err != nil {
		return nil, err
	}
	if p.HotelMapURL, err = safeURL("hotel_map_url", req.HotelMapURL); err != nil {
		return nil, err
	}
	if req.HotelCheckIn != nil && *req.HotelCheckIn != "" {
		t, _ := time.Parse(dateFmt, *req.HotelCheckIn)
		p.HotelCheckIn = &t
	}
	if req.HotelCheckOut != nil && *req.HotelCheckOut != "" {
		t, _ := time.Parse(dateFmt, *req.HotelCheckOut)
		p.HotelCheckOut = &t
	}
	if p.HotelCheckIn != nil && p.HotelCheckOut != nil && p.HotelCheckOut.Before(*p.HotelCheckIn) {
		return nil, validation.Field("hotel_check_out", "gtefield", "check-out must be on or after check-in")
	}
	items := make([]domain.ItineraryItem, 0, len(req.Itinerary))
	for i, it := range req.Itinerary {
		mapURL := it.MapURL
		if mapURL != "" {
			if _, err := safeURL(fmt.Sprintf("itinerary[%d].map_url", i), &mapURL); err != nil {
				return nil, err
			}
		}
		items = append(items, domain.ItineraryItem{Date: it.Date, Time: it.Time, Title: strings.TrimSpace(it.Title),
			Place: strings.TrimSpace(it.Place), MapURL: mapURL, Note: strings.TrimSpace(it.Note)})
	}
	p.Itinerary = jsonb.From(items)

	if err := s.repo.SavePlan(ctx, p); err != nil {
		return nil, err
	}
	saved, err := s.repo.GetPlan(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	// Tell the other side + admins.
	other, link := b.GuideID, guideLink(b.ID.String())
	if viewer == b.GuideID {
		other, link = b.UserID, touristLink(b.ID.String())
	}
	data := map[string]any{"booking_id": b.ID.String(), "code": b.Code}
	s.notify.Push(ctx, []string{realtime.UserRoom(b.UserID), realtime.UserRoom(b.GuideID), realtime.RoleRoom(shared.RoleAdmin)},
		realtime.EvPlanUpdated, data)
	who := "Your guide"
	if viewer == b.UserID {
		who = "The tourist"
	}
	s.notify.Notify(ctx, []uuid.UUID{other}, shared.Notification{Type: "plan.updated", Title: "Trip plan updated",
		Body: fmt.Sprintf("%s updated the trip plan for booking %s.", who, b.Code), Link: link, Data: data})

	r := toPlan(saved, bookingID, true)
	return &r, nil
}

// AdminPlans lists trip plans with their bookings (monitoring).
func (s *Service) AdminPlans(ctx context.Context, adminID uuid.UUID, q dto.TripPlanQuery) (response.Page[dto.TripPlanAdminItem], error) {
	res, err := s.repo.ListPlans(ctx, domain.PlanFilter{Upcoming: q.Upcoming, Params: q.Params})
	if err != nil {
		return response.Page[dto.TripPlanAdminItem]{}, err
	}
	return listing.Map(res, func(p *domain.TripPlan) dto.TripPlanAdminItem {
		item := dto.TripPlanAdminItem{Plan: toPlan(p, p.BookingID, false)}
		if p.Booking != nil {
			item.Booking = toBooking(p.Booking, adminID, shared.RoleAdmin)
		}
		return item
	}), nil
}

// ================================================================== chat

// systemSender is how CholoBangladesh appears in the chat.
func systemSender() *dto.PersonRef {
	return &dto.PersonRef{ID: "system", FullName: "CholoBangladesh", Role: domain.MessageSystem}
}

func toMessage(m *domain.Message, viewer uuid.UUID) dto.MessageResponse {
	r := dto.MessageResponse{ID: m.ID.String(), BookingID: m.BookingID.String(), Kind: domain.MessageUser, Body: m.Body,
		ReadAt: m.ReadAt, CreatedAt: m.CreatedAt}
	if m.Kind == domain.MessageSystem || m.SenderID == nil {
		r.Kind, r.Sender = domain.MessageSystem, systemSender()
		return r
	}
	r.Sender, r.Mine = person(m.Sender, *m.SenderID), *m.SenderID == viewer
	return r
}

// completionMessage is the CholoBangladesh thank-you note posted when a tour is completed.
func completionMessage(b *dto.BookingResponse) string {
	tourist, guide, place := name(b.Tourist), name(b.Guide), "Bangladesh"
	if b.Spot != nil && b.Spot.Title != "" {
		place = b.Spot.Title
	}
	return fmt.Sprintf(`🎉 Tour completed — ধন্যবাদ!

Dear %[1]s, thank you for exploring %[3]s with CholoBangladesh. We hope you are taking home beautiful memories of Bangladesh.

%[2]s, thank you for giving %[1]s such a wonderful tour of %[3]s — guides like you make every journey special.

%[1]s, if you enjoyed it, please leave a review for %[2]s. It helps other travellers find great guides.

This chat is now closed — you can still read your messages here anytime.

With love,
Team CholoBangladesh 🇧🇩`, tourist, guide, place)
}

// postCompletionMessage drops the thank-you note into the chat and pushes it live to both sides.
// Best effort: a failure never undoes the completion.
func (s *Service) postCompletionMessage(ctx context.Context, b *dto.BookingResponse) {
	bookingID, err := uuid.Parse(b.ID)
	if err != nil {
		return
	}
	m := &domain.Message{ID: uuid.Must(uuid.NewV7()), BookingID: bookingID, Kind: domain.MessageSystem,
		Body: completionMessage(b), CreatedAt: time.Now().UTC()}
	if err := s.repo.CreateMessage(ctx, m); err != nil {
		slog.Error("completion chat message failed", "booking", b.ID, "err", err)
		return
	}
	msg := toMessage(m, uuid.Nil)
	rooms := []string{}
	for _, p := range []*dto.PersonRef{b.Tourist, b.Guide} {
		if p != nil {
			if id, err := uuid.Parse(p.ID); err == nil {
				rooms = append(rooms, realtime.UserRoom(id))
			}
		}
	}
	s.notify.Push(ctx, rooms, realtime.EvMessageNew, msg)
}

// Messages returns the chat (newest first page, reversed to oldest→newest) and marks it read for the viewer.
func (s *Service) Messages(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID, q dto.MessagesQuery) ([]dto.MessageResponse, error) {
	b, err := s.load(ctx, viewer, role, bookingID)
	if err != nil {
		return nil, err
	}
	if b.IsParty(viewer) && !b.IsPaid() && b.PaymentStatus != domain.PayRefunded {
		return nil, domain.ErrChatClosed
	}
	var before *time.Time
	if t, err := time.Parse(time.RFC3339Nano, q.Before); err == nil {
		before = &t
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	list, err := s.repo.ListMessages(ctx, bookingID, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageResponse, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, toMessage(&list[i], viewer))
	}
	if b.IsParty(viewer) && before == nil {
		if n, err := s.repo.MarkMessagesRead(ctx, bookingID, viewer); err == nil && n > 0 {
			other := b.GuideID
			if viewer == b.GuideID {
				other = b.UserID
			}
			s.notify.Push(ctx, []string{realtime.UserRoom(other), realtime.UserRoom(viewer)}, realtime.EvMessagesRead,
				map[string]any{"booking_id": bookingID.String(), "reader_id": viewer.String()})
		}
	}
	return out, nil
}

// SendMessage posts a chat message (tourist or guide of a paid booking).
func (s *Service) SendMessage(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID, req dto.MessageRequest) (*dto.MessageResponse, error) {
	b, err := s.load(ctx, viewer, role, bookingID)
	if err != nil {
		return nil, err
	}
	if !b.IsParty(viewer) {
		return nil, domain.ErrReadOnly
	}
	if b.Status == domain.StatusCompleted {
		return nil, domain.ErrChatEnded
	}
	if !b.Live() {
		return nil, domain.ErrChatClosed
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return nil, apperror.ErrValidation
	}
	m := &domain.Message{ID: uuid.Must(uuid.NewV7()), BookingID: bookingID, SenderID: &viewer, Kind: domain.MessageUser,
		Body: body, CreatedAt: time.Now().UTC()}
	if err := s.repo.CreateMessage(ctx, m); err != nil {
		return nil, err
	}
	if viewer == b.UserID {
		m.Sender = b.User
	} else {
		m.Sender = b.Guide
	}
	// Each side gets its own "mine" flag.
	s.notify.Push(ctx, []string{realtime.UserRoom(b.UserID)}, realtime.EvMessageNew, toMessage(m, b.UserID))
	s.notify.Push(ctx, []string{realtime.UserRoom(b.GuideID)}, realtime.EvMessageNew, toMessage(m, b.GuideID))
	r := toMessage(m, viewer)
	return &r, nil
}

// UnreadMessages counts unread chat messages per booking for the badge.
func (s *Service) UnreadMessages(ctx context.Context, viewer uuid.UUID) (*dto.UnreadMessages, error) {
	rows, err := s.repo.UnreadMessages(ctx, viewer)
	if err != nil {
		return nil, err
	}
	out := &dto.UnreadMessages{Bookings: map[string]int64{}}
	for _, r := range rows {
		out.Bookings[r.BookingID.String()] = r.Count
		out.Total += r.Count
	}
	return out, nil
}
