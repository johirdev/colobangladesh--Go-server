// Package phone normalizes and validates phone numbers.
package phone

import (
	"regexp"
	"strings"
)

var (
	bdLocal = regexp.MustCompile(`^01[3-9]\d{8}$`)
	e164    = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)
	cleaner = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "")
)

// NormalizeBD accepts 01XXXXXXXXX, 8801XXXXXXXXX, +8801XXXXXXXXX (with spaces or dashes)
// and returns the canonical E.164 form +8801XXXXXXXXX.
func NormalizeBD(in string) (string, bool) {
	s := cleaner.Replace(strings.TrimSpace(in))
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "00")
	if strings.HasPrefix(s, "880") {
		s = "0" + s[3:]
	}
	if !bdLocal.MatchString(s) {
		return "", false
	}
	return "+88" + s, true
}

// NormalizeE164 validates an international number (used for WhatsApp contact numbers).
func NormalizeE164(in string) (string, bool) {
	s := cleaner.Replace(strings.TrimSpace(in))
	if strings.HasPrefix(s, "00") {
		s = "+" + s[2:]
	}
	if bd, ok := NormalizeBD(s); ok {
		return bd, true
	}
	if !e164.MatchString(s) {
		return "", false
	}
	return s, true
}

// Mask hides the middle digits: +8801712345678 -> +88017*****678
func Mask(p string) string {
	if len(p) < 8 {
		return "****"
	}
	return p[:6] + strings.Repeat("*", len(p)-9) + p[len(p)-3:]
}

// MaskEmail: johirul@gmail.com -> jo*****@gmail.com
func MaskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at < 1 {
		return "****"
	}
	name := e[:at]
	keep := min(2, len(name))
	return name[:keep] + strings.Repeat("*", max(3, len(name)-keep)) + e[at:]
}
