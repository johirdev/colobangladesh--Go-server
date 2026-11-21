// Package sms sends SMS through the BulkSMSBD gateway (Bangladeshi numbers only).
package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cholobangladesh-server/internal/config"
)

type BulkSMSBD struct {
	cfg    config.SMSConfig
	client *http.Client
}

func New(cfg config.SMSConfig) *BulkSMSBD {
	return &BulkSMSBD{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

type apiResponse struct {
	ResponseCode   int    `json:"response_code"`
	SuccessMessage string `json:"success_message"`
	ErrorMessage   string `json:"error_message"`
}

// Send delivers text to an E.164 BD number (+8801XXXXXXXXX).
func (s *BulkSMSBD) Send(ctx context.Context, e164, text string) error {
	if !s.cfg.Enabled {
		slog.Info("sms disabled; message not sent", "to", e164)
		return nil
	}
	form := url.Values{
		"api_key":  {s.cfg.APIKey},
		"type":     {"text"},
		"number":   {strings.TrimPrefix(e164, "+")}, // gateway expects 8801XXXXXXXXX
		"senderid": {s.cfg.SenderID},
		"message":  {text},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.APIURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sms request: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))

	var out apiResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("sms gateway: unexpected response (http %d)", res.StatusCode)
	}
	if out.ResponseCode != 202 {
		return fmt.Errorf("sms gateway: code %d: %s", out.ResponseCode, out.ErrorMessage)
	}
	return nil
}
