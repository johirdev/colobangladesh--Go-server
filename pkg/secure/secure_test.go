package secure

import (
	"bytes"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Encrypt("1990123456789")
	b, _ := c.Encrypt("1990123456789")
	if a == b {
		t.Fatal("ciphertexts must differ (random nonce)")
	}
	got, err := c.Decrypt(a)
	if err != nil || got != "1990123456789" {
		t.Fatalf("decrypt = %q, %v", got, err)
	}
	if _, err := c.Decrypt(a[:len(a)-4] + "AAAA"); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
}

func TestNumericCode(t *testing.T) {
	for i := 0; i < 100; i++ {
		code := NumericCode(6)
		if len(code) != 6 {
			t.Fatalf("len = %d", len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("non digit in %q", code)
			}
		}
	}
}

func TestHMACBindsPurpose(t *testing.T) {
	k := []byte("k")
	if HMACHex(k, "login", "a@b.c", "123456") == HMACHex(k, "reset", "a@b.c", "123456") {
		t.Fatal("codes for different purposes must not collide")
	}
}
