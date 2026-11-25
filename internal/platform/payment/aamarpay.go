// Package payment talks to the aamarPay payment gateway (JSON API).
//
//	Checkout : POST {AAMARPAY_URL}                         -> { result: "true", payment_url }
//	Verify   : GET  {origin}/api/v1/trxcheck/request.php   -> transaction status (server-to-server)
//
// The browser callback (success/fail/cancel) is NEVER trusted: the result of a
// payment is always taken from Verify.
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cholobangladesh-server/internal/config"
)

var (
	ErrDisabled = errors.New("aamarpay: not configured")
	ErrRejected = errors.New("aamarpay: request rejected")
)

type Aamarpay struct {
	initURL   string
	verifyURL string
	storeID   string
	key       string
	sandbox   bool
	hc        *http.Client
}

// New returns nil when STORE_ID / SIGNATURE_KEY / AAMARPAY_URL are not set.
func New(cfg config.PaymentConfig) (*Aamarpay, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	u, err := url.Parse(cfg.AamarpayURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("AAMARPAY_URL is invalid")
	}
	verify := url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/api/v1/trxcheck/request.php"}
	return &Aamarpay{initURL: cfg.AamarpayURL, verifyURL: verify.String(), storeID: cfg.StoreID, key: cfg.SignatureKey,
		sandbox: cfg.Sandbox(), hc: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (a *Aamarpay) Sandbox() bool { return a != nil && a.sandbox }

// Mode is "sandbox" or "live".
func (a *Aamarpay) Mode() string {
	if a.Sandbox() {
		return "sandbox"
	}
	return "live"
}

type Checkout struct {
	TranID     string
	Amount     int
	Currency   string
	Desc       string
	CusName    string
	CusEmail   string
	CusPhone   string
	CusCity    string
	CusCountry string
	SuccessURL string
	FailURL    string
	CancelURL  string
	OptA       string // booking id
	OptB       string // booking code
	OptC       string // payer id
	OptD       string // guide id
}

// Start creates a checkout session and returns aamarPay's hosted payment page URL.
func (a *Aamarpay) Start(ctx context.Context, c Checkout) (string, error) {
	if a == nil {
		return "", ErrDisabled
	}
	body := map[string]string{
		"store_id":      a.storeID,
		"signature_key": a.key,
		"tran_id":       c.TranID,
		"success_url":   c.SuccessURL,
		"fail_url":      c.FailURL,
		"cancel_url":    c.CancelURL,
		"amount":        strconv.Itoa(c.Amount) + ".00",
		"currency":      c.Currency,
		"desc":          c.Desc,
		"cus_name":      c.CusName,
		"cus_email":     c.CusEmail,
		"cus_phone":     c.CusPhone,
		"cus_add1":      c.CusCity,
		"cus_city":      c.CusCity,
		"cus_country":   c.CusCountry,
		"opt_a":         c.OptA,
		"opt_b":         c.OptB,
		"opt_c":         c.OptC,
		"opt_d":         c.OptD,
		"type":          "json",
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.initURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := a.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("aamarpay checkout: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		slog.Warn("aamarpay checkout: unexpected response", "status", res.StatusCode, "body", truncate(string(data), 300))
		return "", ErrRejected
	}
	payURL := Str(out["payment_url"])
	if !isTrue(out["result"]) || payURL == "" {
		slog.Warn("aamarpay checkout rejected", "status", res.StatusCode, "body", truncate(string(data), 300))
		msg := firstNonEmpty(Str(out["message"]), Str(out["error"]), Str(out["errors"]), "checkout rejected")
		return "", fmt.Errorf("%w: %s", ErrRejected, truncate(msg, 200))
	}
	if u, err := url.Parse(payURL); err != nil || u.Scheme != "https" {
		return "", ErrRejected
	}
	return payURL, nil
}

// Verified is aamarPay's view of a transaction (Search Transaction API).
type Verified struct {
	PayStatus        string // "Successful", "Failed", "Pending", ...
	StatusCode       string // "2" = successful
	PgTxnID          string
	MerTxnID         string
	Amount           float64
	StoreAmount      float64
	ProcessingCharge float64
	Currency         string
	BankTrx          string
	CardNumber       string
	PaymentType      string
	StoreID          string
	Raw              map[string]any // safe to store (no signature key)
}

// Successful reports a completed, captured payment.
func (v *Verified) Successful() bool {
	ok := strings.EqualFold(v.PayStatus, "Successful")
	if v.StatusCode != "" {
		ok = ok && v.StatusCode == "2"
	}
	return ok
}

// Failed reports a final failure (anything that is not success / pending / not found).
func (v *Verified) Failed() bool {
	s := strings.ToLower(v.PayStatus)
	return s == "failed" || s == "cancelled" || s == "canceled" || s == "rejected" || s == "expired"
}

// AmountMatches compares the paid amount with what we charged.
func (v *Verified) AmountMatches(expected int) bool {
	return math.Abs(v.Amount-float64(expected)) < 0.01
}

// Verify asks aamarPay for the transaction (server-to-server, never via the browser).
func (a *Aamarpay) Verify(ctx context.Context, tranID string) (*Verified, error) {
	if a == nil {
		return nil, ErrDisabled
	}
	q := url.Values{"request_id": {tranID}, "store_id": {a.storeID}, "signature_key": {a.key}, "type": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.verifyURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aamarpay verify: %w", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		// e.g. a plain string "Invalid Request" when the transaction does not exist yet.
		return &Verified{PayStatus: "Unknown", MerTxnID: tranID, Raw: map[string]any{"message": truncate(string(data), 300)}}, nil
	}
	delete(m, "signature_key")
	delete(m, "store_passwd")
	v := &Verified{
		PayStatus:        Str(m["pay_status"]),
		StatusCode:       Str(m["status_code"]),
		PgTxnID:          clean(m["pg_txnid"]),
		MerTxnID:         Str(m["mer_txnid"]),
		Amount:           num(m["amount"]),
		StoreAmount:      num(m["store_amount"]),
		ProcessingCharge: num(m["processing_charge"]),
		Currency:         Str(m["currency"]),
		BankTrx:          clean(m["bank_trxid"]),
		CardNumber:       firstNonEmpty(clean(m["cardnumber"]), clean(m["card_number"])),
		PaymentType:      clean(m["payment_type"]),
		StoreID:          Str(m["store_id"]),
		Raw:              m,
	}
	if v.PayStatus == "" {
		v.PayStatus = "Unknown"
	}
	return v, nil
}

// StoreMatches is false only when aamarPay reports a different store.
func (a *Aamarpay) StoreMatches(v *Verified) bool {
	return v.StoreID == "" || strings.EqualFold(v.StoreID, a.storeID)
}

// ------------------------------------------------------------------ helpers

// Str converts a JSON scalar to string.
func Str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}

// clean drops aamarPay placeholders such as "Not-Available".
func clean(v any) string {
	s := Str(v)
	switch strings.ToLower(strings.ReplaceAll(s, "-", " ")) {
	case "not available", "null", "n/a":
		return ""
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(t), ",", ""), 64)
		return f
	}
	return 0
}

func isTrue(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
