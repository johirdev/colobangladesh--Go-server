package service

import (
	"context"
	"errors"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
)

const dateLayout = "2006-01-02"

func (s *Service) buildMe(ctx context.Context, u *domain.User) (*dto.MeResponse, error) {
	providers, err := s.Repo.ListIdentityProviders(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	var ver *domain.GuideVerification
	if u.Role == domain.RoleGuide {
		ver, err = s.Repo.FindLatestVerification(ctx, u.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}
	return toMe(u, providers, ver), nil
}

func toMe(u *domain.User, providers []string, ver *domain.GuideVerification) *dto.MeResponse {
	if providers == nil {
		providers = []string{}
	}
	missing := domain.ProfileMissingFields(u)
	next := domain.NextOnboardingStep(u, missing)
	me := &dto.MeResponse{
		ID:              u.ID.String(),
		Role:            string(u.Role),
		Status:          string(u.Status),
		Email:           u.Email,
		EmailVerified:   u.EmailVerified(),
		Phone:           u.Phone,
		PhoneVerified:   u.PhoneVerified(),
		HasPassword:     u.HasPassword(),
		LinkedProviders: providers,
		Profile:         toProfile(u.Profile),
		Onboarding:      dto.Onboarding{Completed: next == "done", NextStep: next, MissingFields: missing},
		LastLoginAt:     u.LastLoginAt,
		CreatedAt:       u.CreatedAt,
	}
	if u.Role == domain.RoleGuide {
		me.GuideProfile = toGuideProfile(u.GuideProfile)
		me.Verification = toVerificationStatus(ver)
	}
	return me
}

func toProfile(p *domain.UserProfile) dto.ProfileResponse {
	if p == nil {
		return dto.ProfileResponse{}
	}
	r := dto.ProfileResponse{
		FullName: p.FullName, AvatarURL: p.AvatarURL, Bio: p.Bio, Gender: p.Gender,
		CountryCode: p.CountryCode, City: p.City, PreferredLanguage: p.PreferredLanguage,
		WhatsappNumber: p.WhatsappNumber,
	}
	if p.DateOfBirth != nil {
		r.DateOfBirth = ptr(p.DateOfBirth.Format(dateLayout))
	}
	return r
}

func toGuideProfile(g *domain.GuideProfile) *dto.GuideProfileResponse {
	if g == nil {
		return nil
	}
	return &dto.GuideProfileResponse{
		Headline: g.Headline, About: g.About,
		Languages: nonNil(g.Languages), ServiceAreas: nonNil(g.ServiceAreas), Specialties: nonNil(g.Specialties),
		ExperienceYears: g.ExperienceYears, HourlyRate: g.HourlyRate, DailyRate: g.DailyRate,
		Currency: g.Currency, MaxGroupSize: g.MaxGroupSize, LicenseNumber: g.LicenseNumber,
		IsAvailable: g.IsAvailable, VerificationStatus: string(g.VerificationStatus), VerifiedAt: g.VerifiedAt,
		RatingAvg: g.RatingAvg, RatingCount: g.RatingCount, TotalTours: g.TotalTours,
	}
}

func toVerificationStatus(v *domain.GuideVerification) *dto.VerificationStatusResponse {
	if v == nil {
		return nil
	}
	return &dto.VerificationStatusResponse{
		ID: v.ID.String(), Status: string(v.Status), NIDLast4: v.NIDLast4,
		RejectionReason: v.RejectionReason, SubmittedAt: v.CreatedAt, ReviewedAt: v.ReviewedAt,
	}
}

func toPublicGuide(u *domain.User, detailed bool) dto.PublicGuideResponse {
	p, g := u.Profile, u.GuideProfile
	r := dto.PublicGuideResponse{
		ID: u.ID.String(), FullName: p.FullName, AvatarURL: p.AvatarURL, City: p.City,
		Headline: g.Headline, Languages: nonNil(g.Languages), ServiceAreas: nonNil(g.ServiceAreas),
		Specialties: nonNil(g.Specialties), ExperienceYears: g.ExperienceYears,
		HourlyRate: g.HourlyRate, DailyRate: g.DailyRate, Currency: g.Currency, MaxGroupSize: g.MaxGroupSize,
		RatingAvg: g.RatingAvg, RatingCount: g.RatingCount, TotalTours: g.TotalTours,
		IsVerified: g.VerificationStatus == domain.VerificationApproved,
		IsLicensed: !blankPtr(g.LicenseNumber), VerifiedAt: g.VerifiedAt, MemberSince: u.CreatedAt,
	}
	if detailed {
		r.About = g.About
	}
	return r
}

func toAdminUser(u *domain.User) dto.AdminUserResponse {
	r := dto.AdminUserResponse{
		ID: u.ID.String(), Role: string(u.Role), Status: string(u.Status),
		Email: u.Email, EmailVerified: u.EmailVerified(), Phone: u.Phone, PhoneVerified: u.PhoneVerified(),
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
	if u.Profile != nil {
		r.FullName = u.Profile.FullName
	}
	if u.GuideProfile != nil {
		r.VerificationStatus = ptr(string(u.GuideProfile.VerificationStatus))
	}
	return r
}

func toAdminVerification(v *domain.GuideVerification) dto.AdminVerificationResponse {
	r := dto.AdminVerificationResponse{
		ID: v.ID.String(), UserID: v.UserID.String(), Status: string(v.Status), NIDLast4: v.NIDLast4,
		RejectionReason: v.RejectionReason, SubmittedAt: v.CreatedAt, ReviewedAt: v.ReviewedAt,
	}
	if v.ReviewedBy != nil {
		r.ReviewedBy = ptr(v.ReviewedBy.String())
	}
	if v.User != nil {
		r.Phone = v.User.Phone
		if v.User.Profile != nil {
			r.FullName = v.User.Profile.FullName
		}
	}
	return r
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func blankPtr(s *string) bool { return s == nil || *s == "" }
