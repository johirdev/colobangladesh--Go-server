// Package secure holds small cryptographic helpers: random tokens, OTP codes,
// HMAC/SHA-256 digests and AES-256-GCM field encryption.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
)

// RandomToken returns a URL-safe random string with n bytes of entropy.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}

// NumericCode returns a uniformly random numeric code of the given length.
func NumericCode(length int) string {
	out := make([]byte, length)
	ten := big.NewInt(10)
	for i := range out {
		n, _ := rand.Int(rand.Reader, ten)
		out[i] = byte('0' + n.Int64())
	}
	return string(out)
}

// SHA256Hex is used for refresh/ticket tokens (high entropy, no salt needed).
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HMACHex is used for OTP codes (low entropy, so a server secret is mixed in).
func HMACHex(secret []byte, parts ...string) string {
	m := hmac.New(sha256.New, secret)
	for _, p := range parts {
		m.Write([]byte(p))
		m.Write([]byte{0})
	}
	return hex.EncodeToString(m.Sum(nil))
}

func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Cipher encrypts sensitive fields (e.g. NID numbers) at rest.
type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
