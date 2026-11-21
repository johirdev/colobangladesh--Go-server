package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/secure"
)

// OTP purposes. A code issued for one purpose can never be used for another.
const (
	purposeRegister = "register"
	purposeLogin    = "login"
	purposeReset    = "reset"
	purposeContact  = "contact"
)

const (
	channelEmail = "email"
	channelSMS   = "sms"
)

type otpRequest struct {
	purpose     string
	channel     string
	key         string // what the code is bound to (canonical email/phone, optionally user-scoped)
	destination string // where it is delivered
	ip          string
	deliver     bool // false = behave identically but send nothing (anti-enumeration)
}

func otpKey(purpose, key string) string { return "otp:" + purpose + ":" + key }

// issueOTP applies every anti-abuse layer, then generates, stores (hashed) and sends a code.
//
// Layers: target block (after too many wrong codes) -> resend cooldown -> per-target hourly cap
// -> per-IP daily SMS cap (SMS-pumping / toll-fraud protection) -> delivery.
func (s *Service) issueOTP(ctx context.Context, r otpRequest) (*dto.OTPSentResponse, error) {
	if ttl, _ := s.Store.TTL(ctx, "otp:block:"+r.destination); ttl > 0 {
		return nil, domain.ErrAccountLocked.WithRetryAfter(ttl)
	}

	cdKey := "otp:cd:" + r.purpose + ":" + r.key
	ok, err := s.Store.SetNX(ctx, cdKey, s.cfg.OTP.ResendCooldown)
	if err != nil {
		return nil, err
	}
	if !ok {
		ttl, _ := s.Store.TTL(ctx, cdKey)
		return nil, domain.ErrOTPCooldown.WithRetryAfter(ttl)
	}

	n, err := s.Store.Incr(ctx, "otp:hr:"+r.destination, time.Hour)
	if err != nil {
		return nil, err
	}
	if int(n) > s.cfg.OTP.MaxPerHour {
		ttl, _ := s.Store.TTL(ctx, "otp:hr:"+r.destination)
		return nil, domain.ErrOTPLimit.WithRetryAfter(ttl)
	}

	if r.channel == channelSMS && r.deliver {
		n, err := s.Store.Incr(ctx, "sms:ip:"+r.ip, 24*time.Hour)
		if err != nil {
			return nil, err
		}
		if int(n) > s.cfg.OTP.SMSMaxPerIPPerDay {
			ttl, _ := s.Store.TTL(ctx, "sms:ip:"+r.ip)
			return nil, domain.ErrOTPLimit.WithRetryAfter(ttl)
		}
	}

	res := &dto.OTPSentResponse{
		Channel:     r.channel,
		Destination: mask(r.channel, r.destination),
		ExpiresIn:   int(s.cfg.OTP.TTL.Seconds()),
		ResendAfter: int(s.cfg.OTP.ResendCooldown.Seconds()),
	}
	if !r.deliver {
		return res, nil
	}

	code := secure.NumericCode(s.cfg.OTP.Length)
	if !s.cfg.Production && s.cfg.OTP.DevCode != "" {
		code = s.cfg.OTP.DevCode // fixed code for frontend development
	}
	hash := secure.HMACHex(s.cfg.OTPSecret, r.purpose, r.key, code)
	if err := s.Store.SaveOTP(ctx, otpKey(r.purpose, r.key), hash, s.cfg.OTP.TTL); err != nil {
		return nil, err
	}

	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.deliverOTP(sendCtx, r, code); err != nil {
		slog.Error("otp delivery failed", "channel", r.channel, "err", err)
		_, _ = s.Store.Del(ctx, otpKey(r.purpose, r.key), cdKey)
		return nil, domain.ErrOTPDelivery
	}
	if !s.cfg.Production && ((r.channel == channelSMS && !s.cfg.SMSEnabled) || (r.channel == channelEmail && !s.cfg.MailEnabled)) {
		slog.Warn("DEV ONLY: otp not delivered (provider disabled)", "purpose", r.purpose, "to", r.destination, "otp", code)
	}
	if !s.cfg.Production && s.cfg.OTP.DevExpose {
		res.DevOTP = code
	}
	return res, nil
}

// verifyOTP checks a code. Wrong codes are counted; after OTP_MAX_ATTEMPTS the code is
// destroyed and the destination is blocked for USER_BLOCK_TIME. Codes are single use.
func (s *Service) verifyOTP(ctx context.Context, purpose, key, destination, code string) error {
	if ttl, _ := s.Store.TTL(ctx, "otp:block:"+destination); ttl > 0 {
		return domain.ErrAccountLocked.WithRetryAfter(ttl)
	}
	k := otpKey(purpose, key)
	rec, err := s.Store.GetOTP(ctx, k)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrOTPExpired
	}
	if err != nil {
		return err
	}

	expected := secure.HMACHex(s.cfg.OTPSecret, purpose, key, code)
	if !secure.Equal(expected, rec.Hash) {
		attempts, err := s.Store.IncrOTPAttempts(ctx, k)
		if err != nil {
			return err
		}
		if attempts >= s.cfg.OTP.MaxAttempts {
			_, _ = s.Store.Del(ctx, k)
			_ = s.Store.Set(ctx, "otp:block:"+destination, "1", s.cfg.BlockTime)
			return domain.ErrAccountLocked.WithRetryAfter(s.cfg.BlockTime)
		}
		return domain.ErrOTPInvalid.WithDetails(map[string]int{"remaining_attempts": s.cfg.OTP.MaxAttempts - attempts})
	}

	// Single use: only the request that actually deletes the key wins (prevents parallel replay).
	n, err := s.Store.Del(ctx, k)
	if err != nil {
		return err
	}
	if n != 1 {
		return domain.ErrOTPExpired
	}
	return nil
}

func (s *Service) deliverOTP(ctx context.Context, r otpRequest, code string) error {
	mins := int(s.cfg.OTP.TTL.Minutes())
	app := s.cfg.AppName
	if r.channel == channelSMS {
		return s.SMS.Send(ctx, r.destination, otpSMS(app, code, mins))
	}
	subject, text, html := otpEmail(app, r.purpose, code, mins)
	return s.Mailer.Send(ctx, r.destination, subject, text, html)
}

func mask(channel, dest string) string {
	if channel == channelEmail {
		return phone.MaskEmail(dest)
	}
	return phone.Mask(dest)
}

// ------------------------------------------------------------------ tickets
//
// A ticket is an opaque, single-use token proving "this contact was verified by OTP".
// It lets the frontend split sign-up / reset into separate screens without
// re-sending the OTP. Only its SHA-256 is stored.

func ticketKey(purpose, token string) string {
	return "ticket:" + purpose + ":" + secure.SHA256Hex(token)
}

func (s *Service) issueTicket(ctx context.Context, purpose, target string, ttl time.Duration) (*dto.VerificationTicketResponse, error) {
	tok := secure.RandomToken(32)
	if err := s.Store.Set(ctx, ticketKey(purpose, tok), target, ttl); err != nil {
		return nil, err
	}
	return &dto.VerificationTicketResponse{Token: tok, ExpiresIn: int(ttl.Seconds())}, nil
}

func (s *Service) peekTicket(ctx context.Context, purpose, token string) (string, error) {
	v, err := s.Store.Get(ctx, ticketKey(purpose, strings.TrimSpace(token)))
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.ErrTicketInvalid
	}
	return v, err
}

func (s *Service) burnTicket(ctx context.Context, purpose, token string) {
	_, _ = s.Store.Del(ctx, ticketKey(purpose, strings.TrimSpace(token)))
}
