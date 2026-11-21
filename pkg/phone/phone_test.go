package phone

import "testing"

func TestNormalizeBD(t *testing.T) {
	ok := map[string]string{
		"01712345678":       "+8801712345678",
		"+8801712345678":    "+8801712345678",
		"8801712345678":     "+8801712345678",
		"008801712345678":   "+8801712345678",
		"017-1234 5678":     "+8801712345678",
		"(+880) 1912345678": "+8801912345678",
	}
	for in, want := range ok {
		got, valid := NormalizeBD(in)
		if !valid || got != want {
			t.Errorf("NormalizeBD(%q) = %q,%v want %q", in, got, valid, want)
		}
	}
	for _, in := range []string{"", "01212345678", "0171234567", "+14155552671", "abc", "017123456789"} {
		if _, valid := NormalizeBD(in); valid {
			t.Errorf("NormalizeBD(%q) should be invalid", in)
		}
	}
}

func TestMask(t *testing.T) {
	if got := Mask("+8801712345678"); got != "+88017*****678" {
		t.Errorf("Mask = %q", got)
	}
	if got := MaskEmail("johirul@gmail.com"); got != "jo*****@gmail.com" {
		t.Errorf("MaskEmail = %q", got)
	}
}
