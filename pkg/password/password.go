// Package password hashes and verifies passwords with bcrypt.
package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type Hasher struct {
	cost  int
	dummy []byte
}

func NewHasher(cost int) *Hasher {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("timing-equalizer-dummy-password"), cost)
	return &Hasher{cost: cost, dummy: dummy}
}

func (h *Hasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	return string(b), err
}

func (h *Hasher) Verify(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}

// Equalize burns the same CPU time as a real comparison so that "user does not
// exist" and "wrong password" are indistinguishable by response time.
func (h *Hasher) Equalize(plain string) {
	_ = bcrypt.CompareHashAndPassword(h.dummy, []byte(plain))
}

var ErrTooLong = errors.New("password too long")
