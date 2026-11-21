// Package token issues and validates short-lived JWT access tokens.
// Refresh tokens are opaque random strings stored hashed in the database.
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	Role         string `json:"role"`
	SessionID    string `json:"sid"`
	TokenVersion int    `json:"tv"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

func NewManager(secret, issuer string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), issuer: issuer, ttl: ttl}
}

func (m *Manager) TTL() time.Duration { return m.ttl }

func (m *Manager) Generate(userID uuid.UUID, role string, sessionID uuid.UUID, tokenVersion int) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(m.ttl)
	claims := Claims{
		Role:         role,
		SessionID:    sessionID.String(),
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   userID.String(),
			Issuer:    m.issuer,
			Audience:  jwt.ClaimStrings{m.issuer + "-api"},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return s, exp, err
}

type Parsed struct {
	UserID       uuid.UUID
	SessionID    uuid.UUID
	Role         string
	TokenVersion int
	ExpiresAt    time.Time
}

func (m *Manager) Parse(tokenStr string) (*Parsed, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.issuer+"-api"),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	sid, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return nil, errors.New("invalid session")
	}
	return &Parsed{UserID: uid, SessionID: sid, Role: claims.Role, TokenVersion: claims.TokenVersion,
		ExpiresAt: claims.ExpiresAt.Time}, nil
}
