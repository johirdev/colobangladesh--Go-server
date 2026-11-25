package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/validation"

	"github.com/lib/pq"
)

const (
	ticketRegisterUser  = "register_user"
	ticketRegisterGuide = "register_guide"
	ticketRegisterLocal = "register_user_phone" // Bangladeshi tourist (BD mobile)
	ticketReset         = "reset_password"
)

// =====================================================================
// Tourist registration: email OTP -> verify -> complete
// =====================================================================

func (s *Service) SendUserRegistrationOTP(ctx context.Context, rawEmail string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	email, ok := normalizeEmail(rawEmail)
	if !ok {
		return nil, validation.Field("email", "email", "must be a valid email address")
	}
	exists, err := s.Repo.EmailExists(ctx, email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrEmailTaken
	}
	return s.issueOTP(ctx, otpRequest{purpose: purposeRegister, channel: channelEmail, key: email,
		destination: email, ip: meta.IP, deliver: true})
}

func (s *Service) VerifyUserRegistrationOTP(ctx context.Context, rawEmail, code string) (*dto.VerificationTicketResponse, error) {
	email, ok := normalizeEmail(rawEmail)
	if !ok {
		return nil, validation.Field("email", "email", "must be a valid email address")
	}
	if err := s.verifyOTP(ctx, purposeRegister, email, email, code); err != nil {
		return nil, err
	}
	return s.issueTicket(ctx, ticketRegisterUser, email, s.cfg.RegistrationTTL)
}

func (s *Service) CompleteUserRegistration(ctx context.Context, req dto.CompleteUserRegistrationRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	email, err := s.peekTicket(ctx, ticketRegisterUser, req.RegistrationToken)
	if err != nil {
		return nil, err
	}

	var pwHash *string
	if req.Password != nil && *req.Password != "" {
		h, err := s.Hasher.Hash(*req.Password)
		if err != nil {
			return nil, err
		}
		pwHash = &h
	}
	lang := "en"
	if req.PreferredLanguage != nil {
		lang = *req.PreferredLanguage
	}
	var country *string
	if req.CountryCode != nil {
		country = ptr(strings.ToUpper(*req.CountryCode))
	}

	t := now()
	u := &domain.User{
		ID: newID(), Role: domain.RoleUser, Status: domain.StatusActive,
		Email: &email, EmailVerifiedAt: &t, PasswordHash: pwHash, TokenVersion: 1,
	}
	u.Profile = &domain.UserProfile{UserID: u.ID, FullName: strings.TrimSpace(req.FullName),
		CountryCode: country, PreferredLanguage: lang}

	if err := s.Repo.CreateUser(ctx, u); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, domain.ErrEmailTaken
		}
		return nil, err
	}
	s.burnTicket(ctx, ticketRegisterUser, req.RegistrationToken)
	s.audit(ctx, &u.ID, &u.ID, domain.EventRegistered, meta, map[string]any{"role": u.Role, "method": "email_otp"})
	s.welcome(u)
	return s.issueAuth(ctx, u, meta, true)
}

// =====================================================================
// Bangladeshi tourist registration: BD phone OTP -> verify -> complete
// (foreign tourists use the email flow above)
// =====================================================================

func (s *Service) SendLocalUserRegistrationOTP(ctx context.Context, rawPhone string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	// Same checks and SMS limits as guide sign-up.
	return s.SendGuideRegistrationOTP(ctx, rawPhone, meta)
}

func (s *Service) VerifyLocalUserRegistrationOTP(ctx context.Context, rawPhone, code string) (*dto.VerificationTicketResponse, error) {
	p, ok := phone.NormalizeBD(rawPhone)
	if !ok {
		return nil, validation.Field("phone", "bd_phone", "must be a valid Bangladeshi mobile number")
	}
	if err := s.verifyOTP(ctx, purposeRegister, p, p, code); err != nil {
		return nil, err
	}
	return s.issueTicket(ctx, ticketRegisterLocal, p, s.cfg.RegistrationTTL)
}

func (s *Service) CompleteLocalUserRegistration(ctx context.Context, req dto.CompleteLocalUserRegistrationRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	p, err := s.peekTicket(ctx, ticketRegisterLocal, req.RegistrationToken)
	if err != nil {
		return nil, err
	}
	var pwHash *string
	if req.Password != nil && *req.Password != "" {
		h, err := s.Hasher.Hash(*req.Password)
		if err != nil {
			return nil, err
		}
		pwHash = &h
	}
	lang := "bn"
	if req.PreferredLanguage != nil {
		lang = *req.PreferredLanguage
	}
	t := now()
	u := &domain.User{
		ID: newID(), Role: domain.RoleUser, Status: domain.StatusActive,
		Phone: &p, PhoneVerifiedAt: &t, PasswordHash: pwHash, TokenVersion: 1,
	}
	u.Profile = &domain.UserProfile{UserID: u.ID, FullName: strings.TrimSpace(req.FullName),
		CountryCode: ptr("BD"), City: trimPtr(req.City), PreferredLanguage: lang}

	if err := s.Repo.CreateUser(ctx, u); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, domain.ErrPhoneTaken
		}
		return nil, err
	}
	s.burnTicket(ctx, ticketRegisterLocal, req.RegistrationToken)
	s.audit(ctx, &u.ID, &u.ID, domain.EventRegistered, meta, map[string]any{"role": u.Role, "method": "phone_otp", "nationality": "BD"})
	return s.issueAuth(ctx, u, meta, true)
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	if v := strings.TrimSpace(*p); v != "" {
		return &v
	}
	return nil
}

// =====================================================================
// Guide registration: BD phone OTP -> verify -> complete
// =====================================================================

func (s *Service) SendGuideRegistrationOTP(ctx context.Context, rawPhone string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	p, ok := phone.NormalizeBD(rawPhone)
	if !ok {
		return nil, validation.Field("phone", "bd_phone", "must be a valid Bangladeshi mobile number")
	}
	exists, err := s.Repo.PhoneExists(ctx, p)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrPhoneTaken
	}
	return s.issueOTP(ctx, otpRequest{purpose: purposeRegister, channel: channelSMS, key: p,
		destination: p, ip: meta.IP, deliver: true})
}

func (s *Service) VerifyGuideRegistrationOTP(ctx context.Context, rawPhone, code string) (*dto.VerificationTicketResponse, error) {
	p, ok := phone.NormalizeBD(rawPhone)
	if !ok {
		return nil, validation.Field("phone", "bd_phone", "must be a valid Bangladeshi mobile number")
	}
	if err := s.verifyOTP(ctx, purposeRegister, p, p, code); err != nil {
		return nil, err
	}
	return s.issueTicket(ctx, ticketRegisterGuide, p, s.cfg.RegistrationTTL)
}

func (s *Service) CompleteGuideRegistration(ctx context.Context, req dto.CompleteGuideRegistrationRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	p, err := s.peekTicket(ctx, ticketRegisterGuide, req.RegistrationToken)
	if err != nil {
		return nil, err
	}
	h, err := s.Hasher.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	t := now()
	u := &domain.User{
		ID: newID(), Role: domain.RoleGuide, Status: domain.StatusActive,
		Phone: &p, PhoneVerifiedAt: &t, PasswordHash: &h, TokenVersion: 1,
	}
	u.Profile = &domain.UserProfile{UserID: u.ID, FullName: strings.TrimSpace(req.FullName),
		CountryCode: ptr("BD"), PreferredLanguage: "bn"}
	u.GuideProfile = newGuideProfile(u)

	if err := s.Repo.CreateUser(ctx, u); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, domain.ErrPhoneTaken
		}
		return nil, err
	}
	s.burnTicket(ctx, ticketRegisterGuide, req.RegistrationToken)
	s.audit(ctx, &u.ID, &u.ID, domain.EventRegistered, meta, map[string]any{"role": u.Role, "method": "phone_otp"})
	return s.issueAuth(ctx, u, meta, true)
}

func newGuideProfile(u *domain.User) *domain.GuideProfile {
	return &domain.GuideProfile{
		UserID: u.ID, Languages: pq.StringArray{"bn"}, ServiceAreas: pq.StringArray{}, Specialties: pq.StringArray{},
		Currency: "BDT", MaxGroupSize: 10, IsAvailable: true, VerificationStatus: domain.VerificationUnverified,
	}
}

// =====================================================================
// Password login with lockout
// =====================================================================

func (s *Service) Login(ctx context.Context, req dto.LoginRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	kind, ident, err := parseIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}
	if err := s.checkLoginLock(ctx, ident, meta.IP); err != nil {
		return nil, err
	}

	u, err := s.findByIdentifier(ctx, kind, ident)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if u == nil || !u.HasPassword() || !contactVerified(u, kind) {
		s.Hasher.Equalize(req.Password) // same timing whether the account exists or not
		return nil, s.loginFailed(ctx, ident, nil, meta)
	}
	if !s.Hasher.Verify(*u.PasswordHash, req.Password) {
		return nil, s.loginFailed(ctx, ident, u, meta)
	}
	if err := accountUsable(u); err != nil {
		return nil, err
	}

	_, _ = s.Store.Del(ctx, "login:fail:"+ident)
	s.markLogin(ctx, u, meta, "password")
	return s.issueAuth(ctx, u, meta, false)
}

func (s *Service) checkLoginLock(ctx context.Context, ident, ip string) error {
	if ttl, _ := s.Store.TTL(ctx, "login:block:"+ident); ttl > 0 {
		return domain.ErrAccountLocked.WithRetryAfter(ttl)
	}
	if ttl, _ := s.Store.TTL(ctx, "login:ipblock:"+ip); ttl > 0 {
		return domain.ErrAccountLocked.WithRetryAfter(ttl)
	}
	return nil
}

// loginFailed counts failures per identifier (USER_MAX_FAILED_ATTEMPTS within
// USER_FAILED_ATTEMPT_WINDOW -> locked for USER_BLOCK_TIME) and per IP (credential stuffing).
func (s *Service) loginFailed(ctx context.Context, ident string, u *domain.User, meta domain.RequestMeta) error {
	max := s.cfg.MaxFailedAttempts
	n, err := s.Store.Incr(ctx, "login:fail:"+ident, s.cfg.FailWindow)
	if err != nil {
		return err
	}
	ipN, _ := s.Store.Incr(ctx, "login:ipfail:"+meta.IP, time.Hour)
	if int(ipN) >= max*6 {
		_ = s.Store.Set(ctx, "login:ipblock:"+meta.IP, "1", s.cfg.BlockTime)
	}

	if u != nil {
		s.audit(ctx, &u.ID, nil, domain.EventLoginFailed, meta, map[string]any{"attempt": n})
	}
	if int(n) >= max {
		_ = s.Store.Set(ctx, "login:block:"+ident, "1", s.cfg.BlockTime)
		_, _ = s.Store.Del(ctx, "login:fail:"+ident)
		if u != nil {
			s.audit(ctx, &u.ID, nil, domain.EventAccountLocked, meta, map[string]any{"block_seconds": s.cfg.BlockTime.Seconds()})
		}
		return domain.ErrAccountLocked.WithRetryAfter(s.cfg.BlockTime)
	}
	return domain.ErrInvalidCredentials.WithDetails(map[string]int{"remaining_attempts": max - int(n)})
}

func (s *Service) markLogin(ctx context.Context, u *domain.User, meta domain.RequestMeta, method string) {
	t := now()
	u.LastLoginAt = &t
	_ = s.Repo.UpdateUser(ctx, u.ID, map[string]any{"last_login_at": t, "last_login_ip": meta.IP})
	s.audit(ctx, &u.ID, &u.ID, domain.EventLoginSuccess, meta, map[string]any{"method": method})
}

// =====================================================================
// Passwordless login (email OTP for tourists, SMS OTP for guides)
// =====================================================================

// SendLoginOTP always answers the same way whether or not the account exists.
func (s *Service) SendLoginOTP(ctx context.Context, identifier string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	kind, ident, err := parseIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	u, err := s.findByIdentifier(ctx, kind, ident)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	deliver := u != nil && u.IsActive() && contactVerified(u, kind)
	return s.issueOTP(ctx, otpRequest{purpose: purposeLogin, channel: channelOf(kind), key: ident,
		destination: ident, ip: meta.IP, deliver: deliver})
}

func (s *Service) VerifyLoginOTP(ctx context.Context, req dto.IdentifierOTPRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	kind, ident, err := parseIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}
	if err := s.verifyOTP(ctx, purposeLogin, ident, ident, req.OTP); err != nil {
		return nil, err
	}
	u, err := s.findByIdentifier(ctx, kind, ident)
	if err != nil {
		return nil, notFound(err, domain.ErrOTPExpired)
	}
	if err := accountUsable(u); err != nil {
		return nil, err
	}
	_, _ = s.Store.Del(ctx, "login:fail:"+ident, "login:block:"+ident) // ownership proven
	s.markLogin(ctx, u, meta, "otp")
	return s.issueAuth(ctx, u, meta, false)
}

func channelOf(k identifierKind) string {
	if k == kindEmail {
		return channelEmail
	}
	return channelSMS
}

// =====================================================================
// Google / Apple sign-in (tourists)
// =====================================================================

func (s *Service) OAuthLogin(ctx context.Context, provider string, req dto.OAuthRequest, meta domain.RequestMeta) (*dto.AuthResponse, error) {
	ext, err := s.Identity.Verify(ctx, provider, req.IDToken, req.Nonce)
	switch {
	case errors.Is(err, domain.ErrIdentityProviderDisabled):
		return nil, domain.ErrOAuthDisabled
	case err != nil:
		return nil, domain.ErrOAuthInvalid.Wrap(err)
	}

	// 1) Known identity -> log in.
	ident, err := s.Repo.FindIdentity(ctx, provider, ext.Subject)
	if err == nil {
		u, err := s.Repo.FindUserByID(ctx, ident.UserID)
		if err != nil {
			return nil, notFound(err, domain.ErrAccountDeleted)
		}
		if err := accountUsable(u); err != nil {
			return nil, err
		}
		_ = s.Repo.TouchIdentity(ctx, ident.ID)
		s.markLogin(ctx, u, meta, provider)
		return s.issueAuth(ctx, u, meta, false)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Linking/creating by email is only safe when the provider verified the email.
	email, ok := normalizeEmail(ext.Email)
	if !ok || !ext.EmailVerified {
		return nil, domain.ErrOAuthEmailUnverified
	}

	// 2) Existing account with the same verified email -> link identity.
	u, err := s.Repo.FindUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if u != nil {
		if err := accountUsable(u); err != nil {
			return nil, err
		}
		if err := s.Repo.CreateIdentity(ctx, &domain.AuthIdentity{ID: newID(), UserID: u.ID, Provider: provider,
			ProviderSubject: ext.Subject, Email: &email, CreatedAt: now(), LastUsedAt: now()}); err != nil {
			return nil, err
		}
		if !u.EmailVerified() {
			t := now()
			u.EmailVerifiedAt = &t
			_ = s.Repo.UpdateUser(ctx, u.ID, map[string]any{"email_verified_at": t})
		}
		s.audit(ctx, &u.ID, &u.ID, domain.EventOAuthLinked, meta, map[string]any{"provider": provider})
		s.markLogin(ctx, u, meta, provider)
		return s.issueAuth(ctx, u, meta, false)
	}

	// 3) New tourist account.
	name := strings.TrimSpace(ext.Name)
	if name == "" {
		name = strings.TrimSpace(req.FullName)
	}
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	t := now()
	u = &domain.User{ID: newID(), Role: domain.RoleUser, Status: domain.StatusActive,
		Email: &email, EmailVerifiedAt: &t, TokenVersion: 1}
	u.Profile = &domain.UserProfile{UserID: u.ID, FullName: truncate(name, 100), PreferredLanguage: "en"}
	if strings.HasPrefix(ext.Picture, "https://") {
		u.Profile.AvatarURL = &ext.Picture
	}

	err = s.Repo.Tx(ctx, func(r domain.Repository) error {
		if err := r.CreateUser(ctx, u); err != nil {
			return err
		}
		return r.CreateIdentity(ctx, &domain.AuthIdentity{ID: newID(), UserID: u.ID, Provider: provider,
			ProviderSubject: ext.Subject, Email: &email, CreatedAt: t, LastUsedAt: t})
	})
	if errors.Is(err, domain.ErrDuplicate) {
		return nil, domain.ErrEmailTaken // concurrent sign-up race
	}
	if err != nil {
		return nil, err
	}
	s.audit(ctx, &u.ID, &u.ID, domain.EventRegistered, meta, map[string]any{"role": u.Role, "method": provider})
	s.welcome(u)
	s.markLogin(ctx, u, meta, provider)
	return s.issueAuth(ctx, u, meta, true)
}

// =====================================================================
// Forgot password: OTP -> verify -> reset_token -> new password
// =====================================================================

func (s *Service) ForgotPassword(ctx context.Context, identifier string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	kind, ident, err := parseIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	u, err := s.findByIdentifier(ctx, kind, ident)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	deliver := u != nil && u.IsActive() && contactVerified(u, kind)
	return s.issueOTP(ctx, otpRequest{purpose: purposeReset, channel: channelOf(kind), key: ident,
		destination: ident, ip: meta.IP, deliver: deliver})
}

func (s *Service) VerifyResetOTP(ctx context.Context, req dto.IdentifierOTPRequest) (*dto.VerificationTicketResponse, error) {
	_, ident, err := parseIdentifier(req.Identifier)
	if err != nil {
		return nil, err
	}
	if err := s.verifyOTP(ctx, purposeReset, ident, ident, req.OTP); err != nil {
		return nil, err
	}
	return s.issueTicket(ctx, ticketReset, ident, s.cfg.PasswordResetTTL)
}

func (s *Service) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest, meta domain.RequestMeta) error {
	ident, err := s.peekTicket(ctx, ticketReset, req.ResetToken)
	if err != nil {
		return err
	}
	kind, ident, err := parseIdentifier(ident)
	if err != nil {
		return domain.ErrTicketInvalid
	}
	u, err := s.findByIdentifier(ctx, kind, ident)
	if err != nil {
		return notFound(err, domain.ErrTicketInvalid)
	}
	if err := accountUsable(u); err != nil {
		return err
	}
	h, err := s.Hasher.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.Repo.UpdateUser(ctx, u.ID, map[string]any{"password_hash": h}); err != nil {
		return err
	}
	s.burnTicket(ctx, ticketReset, req.ResetToken)
	if err := s.revokeEverything(ctx, u.ID); err != nil {
		return err
	}
	_, _ = s.Store.Del(ctx, "login:fail:"+ident, "login:block:"+ident)
	s.audit(ctx, &u.ID, &u.ID, domain.EventPasswordReset, meta, nil)
	s.notifySecurity(u, "Your password was reset", "Your password was just reset and all devices were signed out. "+
		"If this wasn't you, contact support immediately.")
	return nil
}

// ------------------------------------------------------------------ notifications

func (s *Service) welcome(u *domain.User) {
	if !u.EmailVerified() {
		return
	}
	to, name := *u.Email, u.Profile.FullName
	s.async("welcome_email", func(ctx context.Context) error {
		subj, text, html := simpleEmail(s.cfg.AppName, "Welcome, "+name+"!",
			"Your account is ready. Start exploring Bangladesh with trusted local guides.")
		return s.Mailer.Send(ctx, to, subj, text, html)
	})
}

// notifySecurity informs the user about a sensitive change on their verified channel.
func (s *Service) notifySecurity(u *domain.User, title, body string) {
	switch {
	case u.EmailVerified():
		to := *u.Email
		s.async("security_email", func(ctx context.Context) error {
			subj, text, html := simpleEmail(s.cfg.AppName, title, body)
			return s.Mailer.Send(ctx, to, subj, text, html)
		})
	case u.PhoneVerified():
		to := *u.Phone
		s.async("security_sms", func(ctx context.Context) error {
			return s.SMS.Send(ctx, to, s.cfg.AppName+": "+body)
		})
	}
}
