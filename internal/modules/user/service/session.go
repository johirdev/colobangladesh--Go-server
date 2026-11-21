package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/secure"
	"cholobangladesh-server/pkg/token"

	"github.com/google/uuid"
)

// Within this window a re-used (already rotated) refresh token is treated as a
// benign race between two tabs instead of token theft.
const rotationGrace = 30 * time.Second

const authStateTTL = 5 * time.Minute

// issueAuth creates a new device session and returns the token pair + profile.
func (s *Service) issueAuth(ctx context.Context, u *domain.User, meta domain.RequestMeta, isNew bool) (*dto.AuthResponse, error) {
	refresh := secure.RandomToken(32)
	sess := &domain.Session{
		ID:               newID(),
		UserID:           u.ID,
		RefreshTokenHash: secure.SHA256Hex(refresh),
		UserAgent:        truncate(meta.UserAgent, 255),
		IPAddress:        meta.IP,
		CreatedAt:        now(),
		LastUsedAt:       now(),
		ExpiresAt:        now().Add(s.cfg.RefreshTTL),
	}
	if err := s.Repo.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	return s.tokenResponse(ctx, u, sess.ID, refresh, isNew)
}

func (s *Service) tokenResponse(ctx context.Context, u *domain.User, sid uuid.UUID, refresh string, isNew bool) (*dto.AuthResponse, error) {
	access, _, err := s.Tokens.Generate(u.ID, string(u.Role), sid, u.TokenVersion)
	if err != nil {
		return nil, err
	}
	me, err := s.buildMe(ctx, u)
	if err != nil {
		return nil, err
	}
	return &dto.AuthResponse{
		AccessToken:      access,
		RefreshToken:     refresh,
		TokenType:        "Bearer",
		ExpiresIn:        int(s.Tokens.TTL().Seconds()),
		RefreshExpiresIn: int(s.cfg.RefreshTTL.Seconds()),
		IsNewUser:        isNew,
		User:             me,
	}, nil
}

// Refresh rotates the refresh token. Presenting an already-rotated token outside the
// grace window means it was stolen: the whole session is revoked.
func (s *Service) Refresh(ctx context.Context, refresh string, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	hash := secure.SHA256Hex(strings.TrimSpace(refresh))

	sess, err := s.Repo.FindSessionByTokenHash(ctx, hash)
	if errors.Is(err, domain.ErrNotFound) {
		prev, perr := s.Repo.FindSessionByPreviousHash(ctx, hash)
		if perr == nil && prev.RevokedAt == nil {
			if prev.RotatedAt != nil && time.Since(*prev.RotatedAt) < rotationGrace {
				return nil, domain.ErrRefreshRotated
			}
			_, _ = s.Repo.RevokeSession(ctx, prev.UserID, prev.ID)
			s.blacklistSession(ctx, prev.ID)
			s.audit(ctx, &prev.UserID, nil, domain.EventRefreshReuse, meta, map[string]any{"session_id": prev.ID})
			slog.Warn("refresh token reuse detected; session revoked", "user_id", prev.UserID, "session_id", prev.ID)
		}
		return nil, domain.ErrRefreshInvalid
	}
	if err != nil {
		return nil, err
	}
	if sess.RevokedAt != nil || now().After(sess.ExpiresAt) {
		return nil, domain.ErrRefreshInvalid
	}

	u, err := s.Repo.FindUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, notFound(err, domain.ErrRefreshInvalid)
	}
	if err := accountUsable(u); err != nil {
		_, _ = s.Repo.RevokeSession(ctx, u.ID, sess.ID)
		return nil, err
	}

	next := secure.RandomToken(32)
	ok, err := s.Repo.RotateSession(ctx, sess.ID, hash, secure.SHA256Hex(next), now().Add(s.cfg.RefreshTTL),
		domain.RequestMeta{IP: meta.IP, UserAgent: truncate(meta.UserAgent, 255)})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrRefreshRotated
	}
	return s.tokenResponse(ctx, u, sess.ID, next, false)
}

// Logout revokes the current device session immediately (access token included).
func (s *Service) Logout(ctx context.Context, userID, sid uuid.UUID) error {
	if _, err := s.Repo.RevokeSession(ctx, userID, sid); err != nil {
		return err
	}
	s.blacklistSession(ctx, sid)
	return nil
}

// LogoutAll signs the user out everywhere by bumping the token version.
func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID, meta domain.RequestMeta) error {
	if err := s.revokeEverything(ctx, userID); err != nil {
		return err
	}
	s.audit(ctx, &userID, &userID, domain.EventLogoutAll, meta, nil)
	return nil
}

func (s *Service) revokeEverything(ctx context.Context, userID uuid.UUID) error {
	if _, err := s.Repo.RevokeAllSessions(ctx, userID, nil); err != nil {
		return err
	}
	if _, err := s.Repo.BumpTokenVersion(ctx, userID); err != nil {
		return err
	}
	s.invalidateAuthState(ctx, userID)
	return nil
}

func (s *Service) revokeOtherSessions(ctx context.Context, userID, keep uuid.UUID) error {
	ids, err := s.Repo.RevokeAllSessions(ctx, userID, &keep)
	if err != nil {
		return err
	}
	for _, id := range ids {
		s.blacklistSession(ctx, id)
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, userID, current uuid.UUID) ([]dto.SessionResponse, error) {
	list, err := s.Repo.ListActiveSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SessionResponse, 0, len(list))
	for _, x := range list {
		out = append(out, dto.SessionResponse{
			ID: x.ID.String(), UserAgent: x.UserAgent, IPAddress: x.IPAddress, Current: x.ID == current,
			CreatedAt: x.CreatedAt, LastUsedAt: x.LastUsedAt, ExpiresAt: x.ExpiresAt,
		})
	}
	return out, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID, sid uuid.UUID) error {
	ok, err := s.Repo.RevokeSession(ctx, userID, sid)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrSessionNotFound
	}
	s.blacklistSession(ctx, sid)
	return nil
}

// ------------------------------------------------------------------ access validation

func (s *Service) blacklistSession(ctx context.Context, sid uuid.UUID) {
	if err := s.Store.Set(ctx, "revoked:sid:"+sid.String(), "1", s.Tokens.TTL()+time.Minute); err != nil {
		slog.Warn("blacklist session failed", "err", err)
	}
}

func (s *Service) invalidateAuthState(ctx context.Context, userID uuid.UUID) {
	_, _ = s.Store.Del(ctx, "authstate:"+userID.String())
}

func (s *Service) authState(ctx context.Context, userID uuid.UUID) (int, domain.Status, error) {
	key := "authstate:" + userID.String()
	if v, err := s.Store.Get(ctx, key); err == nil {
		if tv, st, ok := strings.Cut(v, "|"); ok {
			if n, err := strconv.Atoi(tv); err == nil {
				return n, domain.Status(st), nil
			}
		}
	}
	tv, st, err := s.Repo.AuthState(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	_ = s.Store.Set(ctx, key, fmt.Sprintf("%d|%s", tv, st), authStateTTL)
	return tv, st, nil
}

// ValidateAccess implements middleware.SessionValidator.
func (s *Service) ValidateAccess(ctx context.Context, t *token.Parsed) error {
	if _, err := s.Store.Get(ctx, "revoked:sid:"+t.SessionID.String()); err == nil {
		return domain.ErrSessionRevoked
	}
	tv, st, err := s.authState(ctx, t.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrSessionRevoked
	}
	if err != nil {
		return err
	}
	switch st {
	case domain.StatusActive:
	case domain.StatusSuspended:
		return domain.ErrAccountSuspended
	default:
		return domain.ErrSessionRevoked
	}
	if tv != t.TokenVersion {
		return domain.ErrSessionRevoked
	}
	return nil
}

// StartMaintenance periodically deletes long-expired session rows.
func (s *Service) StartMaintenance(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n, err := s.Repo.PurgeExpiredSessions(ctx, now().Add(-7*24*time.Hour))
				if err != nil {
					slog.Warn("session purge failed", "err", err)
				} else if n > 0 {
					slog.Info("expired sessions purged", "count", n)
				}
			}
		}
	}()
}
