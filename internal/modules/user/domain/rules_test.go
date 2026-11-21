package domain

import (
	"slices"
	"testing"
	"time"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	if a := Age(time.Date(2008, 11, 22, 0, 0, 0, 0, time.UTC), now); a != 17 {
		t.Fatalf("age = %d, want 17", a)
	}
	if a := Age(time.Date(2008, 11, 20, 0, 0, 0, 0, time.UTC), now); a != 18 {
		t.Fatalf("age = %d, want 18", a)
	}
}

func TestOnboardingTourist(t *testing.T) {
	u := &User{Role: RoleUser, Profile: &UserProfile{FullName: "Emma"}}
	m := ProfileMissingFields(u)
	if !slices.Equal(m, []string{"country_code"}) || NextOnboardingStep(u, m) != "complete_profile" {
		t.Fatalf("missing = %v", m)
	}
	cc := "GB"
	u.Profile.CountryCode = &cc
	if m := ProfileMissingFields(u); len(m) != 0 || NextOnboardingStep(u, m) != "done" {
		t.Fatalf("expected done, missing = %v", m)
	}
}

func TestOnboardingGuide(t *testing.T) {
	s := func(v string) *string { return &v }
	dob := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	rate := 800
	u := &User{Role: RoleGuide,
		Profile: &UserProfile{FullName: "Rahim", AvatarURL: s("x"), DateOfBirth: &dob, City: s("Khulna")},
		GuideProfile: &GuideProfile{Headline: s("h"), About: s("a"), Languages: []string{"bn"},
			ServiceAreas: []string{"khulna"}, HourlyRate: &rate, VerificationStatus: VerificationUnverified}}

	steps := map[VerificationStatus]string{
		VerificationUnverified: "submit_verification",
		VerificationPending:    "awaiting_review",
		VerificationRejected:   "resubmit_verification",
		VerificationApproved:   "done",
	}
	for st, want := range steps {
		u.GuideProfile.VerificationStatus = st
		if got := NextOnboardingStep(u, ProfileMissingFields(u)); got != want {
			t.Errorf("%s: got %s want %s", st, got, want)
		}
	}
	u.GuideProfile.HourlyRate = nil
	if m := ProfileMissingFields(u); !slices.Equal(m, []string{"rate"}) {
		t.Fatalf("missing = %v", m)
	}
}
