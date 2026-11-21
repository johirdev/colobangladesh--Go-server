package domain

import (
	"strings"
	"time"
)

// GuideSpecialties is the controlled vocabulary used for guide filters.
var GuideSpecialties = []string{
	"history_culture", "food", "nature_wildlife", "adventure_trekking", "photography",
	"religious_heritage", "city_tour", "river_cruise", "tea_garden", "beach",
	"tribal_culture", "shopping",
}

const (
	MinGuideAge = 18
	MinUserAge  = 13
)

func blank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

// ProfileMissingFields lists what must be filled before a user is considered onboarded.
// Tourists only need the basics; guides need everything a tourist would want to see
// before booking, and everything an admin needs to review their KYC.
func ProfileMissingFields(u *User) []string {
	missing := []string{}
	p := u.Profile
	if p == nil {
		return []string{"full_name"}
	}
	if strings.TrimSpace(p.FullName) == "" {
		missing = append(missing, "full_name")
	}

	switch u.Role {
	case RoleUser:
		if blank(p.CountryCode) {
			missing = append(missing, "country_code")
		}
	case RoleGuide:
		if blank(p.AvatarURL) {
			missing = append(missing, "avatar")
		}
		if p.DateOfBirth == nil {
			missing = append(missing, "date_of_birth")
		}
		if blank(p.City) {
			missing = append(missing, "city")
		}
		g := u.GuideProfile
		if g == nil {
			return append(missing, "headline", "about", "languages", "service_areas", "rate")
		}
		if blank(g.Headline) {
			missing = append(missing, "headline")
		}
		if blank(g.About) {
			missing = append(missing, "about")
		}
		if len(g.Languages) == 0 {
			missing = append(missing, "languages")
		}
		if len(g.ServiceAreas) == 0 {
			missing = append(missing, "service_areas")
		}
		if g.HourlyRate == nil && g.DailyRate == nil {
			missing = append(missing, "rate")
		}
	}
	return missing
}

// Age returns full years between dob and now.
func Age(dob, now time.Time) int {
	years := now.Year() - dob.Year()
	if now.YearDay() < dob.YearDay() {
		years--
	}
	return years
}

// NextOnboardingStep computes what the frontend should show next.
func NextOnboardingStep(u *User, missing []string) string {
	if u.Role == RoleAdmin || u.Role == RoleModerator {
		return "done"
	}
	if len(missing) > 0 {
		return "complete_profile"
	}
	if u.Role == RoleGuide && u.GuideProfile != nil {
		switch u.GuideProfile.VerificationStatus {
		case VerificationUnverified:
			return "submit_verification"
		case VerificationPending:
			return "awaiting_review"
		case VerificationRejected:
			return "resubmit_verification"
		}
	}
	return "done"
}
