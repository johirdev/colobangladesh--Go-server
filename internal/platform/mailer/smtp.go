// Package mailer sends transactional email over SMTP (Gmail, SES, Brevo, ...).
package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cholobangladesh-server/internal/config"

	"github.com/wneessen/go-mail"
)

type SMTP struct {
	cfg config.MailConfig
}

func New(cfg config.MailConfig) *SMTP { return &SMTP{cfg: cfg} }

func (s *SMTP) Send(ctx context.Context, to, subject, text, html string) error {
	if !s.cfg.Enabled {
		slog.Info("email disabled; message not sent", "to", to, "subject", subject)
		return nil
	}

	msg := mail.NewMsg()
	if err := msg.FromFormat(s.cfg.FromName, s.cfg.FromEmail); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("mail to: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, text)
	if html != "" {
		msg.AddAlternativeString(mail.TypeTextHTML, html)
	}

	opts := []mail.Option{
		mail.WithPort(s.cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(s.cfg.Username),
		mail.WithPassword(s.cfg.Password),
		mail.WithTimeout(10 * time.Second),
	}
	if s.cfg.Port == 465 {
		opts = append(opts, mail.WithSSLPort(false))
	} else {
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}

	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("mail client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("mail send: %w", err)
	}
	return nil
}
