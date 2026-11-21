// Package captcha verifies Cloudflare Turnstile tokens (bot protection on
// OTP / login / password-reset endpoints).
package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Turnstile struct {
	enabled bool
	secret  string
	client  *http.Client
}

func NewTurnstile(enabled bool, secret string) *Turnstile {
	return &Turnstile{enabled: enabled, secret: secret, client: &http.Client{Timeout: 5 * time.Second}}
}

func (t *Turnstile) Enabled() bool { return t.enabled }

func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	if !t.enabled {
		return true, nil
	}
	if token == "" || len(token) > 2048 {
		return false, nil
	}
	form := url.Values{"secret": {t.secret}, "response": {token}, "remoteip": {remoteIP}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := t.client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Success, nil
}
