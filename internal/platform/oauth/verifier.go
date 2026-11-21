// Package oauth verifies Google and Apple ID tokens obtained by the frontend
// (Google Identity Services / Sign in with Apple) against the providers' JWKS.
package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	ProviderGoogle = "google"
	ProviderApple  = "apple"
)

var (
	ErrProviderDisabled = errors.New("oauth provider is not configured")
	ErrInvalidToken     = errors.New("invalid id token")
)

type Identity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

type provider struct {
	verifier  *oidc.IDTokenVerifier
	issuers   []string
	clientIDs []string
}

type Verifier struct {
	providers map[string]*provider
}

func NewVerifier(googleClientIDs, appleClientIDs []string) *Verifier {
	v := &Verifier{providers: map[string]*provider{}}
	ctx := context.Background()
	cfg := &oidc.Config{SkipClientIDCheck: true, SkipIssuerCheck: true} // checked manually below (multi client IDs)

	if len(googleClientIDs) > 0 {
		v.providers[ProviderGoogle] = &provider{
			verifier:  oidc.NewVerifier("https://accounts.google.com", oidc.NewRemoteKeySet(ctx, "https://www.googleapis.com/oauth2/v3/certs"), cfg),
			issuers:   []string{"https://accounts.google.com", "accounts.google.com"},
			clientIDs: googleClientIDs,
		}
	}
	if len(appleClientIDs) > 0 {
		v.providers[ProviderApple] = &provider{
			verifier:  oidc.NewVerifier("https://appleid.apple.com", oidc.NewRemoteKeySet(ctx, "https://appleid.apple.com/auth/keys"), cfg),
			issuers:   []string{"https://appleid.apple.com"},
			clientIDs: appleClientIDs,
		}
	}
	return v
}

type claims struct {
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // Google: bool, Apple: "true"/"false" string
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Nonce         string `json:"nonce"`
}

// Verify checks signature, expiry, issuer, audience and (optionally) nonce.
// nonce may be the raw nonce; Apple native SDKs put SHA-256(nonce) in the token.
func (v *Verifier) Verify(ctx context.Context, providerName, rawToken, nonce string) (*Identity, error) {
	p, ok := v.providers[providerName]
	if !ok {
		return nil, ErrProviderDisabled
	}
	tok, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, errors.Join(ErrInvalidToken, err)
	}
	if !slices.Contains(p.issuers, tok.Issuer) {
		return nil, ErrInvalidToken
	}
	if !slices.ContainsFunc(tok.Audience, func(a string) bool { return slices.Contains(p.clientIDs, a) }) {
		return nil, ErrInvalidToken
	}

	var c claims
	if err := tok.Claims(&c); err != nil {
		return nil, errors.Join(ErrInvalidToken, err)
	}
	if nonce != "" {
		sum := sha256.Sum256([]byte(nonce))
		if c.Nonce != nonce && c.Nonce != hex.EncodeToString(sum[:]) {
			return nil, ErrInvalidToken
		}
	}

	return &Identity{
		Provider:      providerName,
		Subject:       tok.Subject,
		Email:         c.Email,
		EmailVerified: truthy(c.EmailVerified),
		Name:          c.Name,
		Picture:       c.Picture,
	}, nil
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(t)
		return b
	}
	return false
}
