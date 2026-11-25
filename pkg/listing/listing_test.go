package listing

import (
	"testing"

	"github.com/google/uuid"
)

var spec = Spec{
	Sorts:       map[string]string{"newest": "created_at DESC", "title": "title"},
	DefaultSort: "newest",
}

func TestNormalizeDefaults(t *testing.T) {
	p, err := spec.Normalize(Params{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Page != 1 || p.Limit != DefaultLimit || p.Sort != "newest" {
		t.Fatalf("defaults: %+v", p)
	}
	if p.Offset() != 0 {
		t.Fatalf("offset %d", p.Offset())
	}
}

func TestNormalizeCapsLimitAndOffsets(t *testing.T) {
	s := spec
	s.MaxLimit = 50
	p, _ := s.Normalize(Params{Page: 3, Limit: 500, Q: "  sajek "})
	if p.Limit != 50 || p.Offset() != 100 || p.Q != "sajek" {
		t.Fatalf("got %+v offset=%d", p, p.Offset())
	}
}

func TestNormalizeRejectsUnknownSort(t *testing.T) {
	if _, err := spec.Normalize(Params{Sort: "drop table"}); err == nil {
		t.Fatal("expected a validation error for an unknown sort")
	}
	if _, err := spec.Normalize(Params{Sort: "title"}); err != nil {
		t.Fatal(err)
	}
}

func TestPresentSkipsEmptyValues(t *testing.T) {
	f, zero := false, 0
	cases := []struct {
		v    any
		want bool
	}{
		{nil, false}, {"", false}, {0, false}, {uuid.Nil, false}, {(*int)(nil), false}, {[]int{}, false},
		{"x", true}, {7, true}, {uuid.New(), true}, {&f, true}, {&zero, true}, {[]int{1}, true},
	}
	for _, c := range cases {
		if _, ok := present(c.v); ok != c.want {
			t.Errorf("present(%#v) = %v, want %v", c.v, ok, c.want)
		}
	}
	if v, _ := present(&f); v != false {
		t.Errorf("pointer not dereferenced: %#v", v)
	}
}

func TestLikeEscapes(t *testing.T) {
	if got := Like(` 50%_off\ `); got != `%50\%\_off\\%` {
		t.Fatalf("Like = %q", got)
	}
}
