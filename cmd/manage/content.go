package main

import (
	"context"
	"fmt"
	"strings"

	seeddata "cholobangladesh-server/internal/seed"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// seedContent loads Bangladesh geo data, the 20 default tourist spots, demo payment
// numbers and demo gigs for the seeded guides. Everything is idempotent.
func seedContent(ctx context.Context, db *gorm.DB) error {
	c, err := seeddata.Geo(ctx, db)
	if err != nil {
		return err
	}
	fmt.Printf("✔ geo: %d divisions, %d districts, %d upazilas/thanas\n", c.Divisions, c.Districts, c.Upazilas)

	var moderator *uuid.UUID
	var mod struct{ ID uuid.UUID }
	if db.WithContext(ctx).Raw("SELECT id FROM users WHERE email = ?", "moderator@cholobangladesh.com").Scan(&mod).Error == nil &&
		mod.ID != uuid.Nil {
		moderator = &mod.ID
	}
	created, skipped, err := seeddata.Spots(ctx, db, moderator)
	if err != nil {
		return err
	}
	fmt.Printf("✔ tourist spots: %d created, %d skipped (already exist)\n", created, skipped)

	if err := seedPaymentMethods(ctx, db); err != nil {
		return err
	}
	if err := seedGigs(ctx, db); err != nil {
		return err
	}
	fmt.Println("\nseed done.")
	return nil
}

func seedPaymentMethods(ctx context.Context, db *gorm.DB) error {
	var n int64
	if err := db.WithContext(ctx).Table("payment_methods").Count(&n).Error; err != nil || n > 0 {
		if n > 0 {
			fmt.Printf("• payment methods: %d exist, skipped\n", n)
		}
		return err
	}
	methods := []struct{ provider, number, kind, instructions string }{
		{"bkash", "01700000000", "personal", "DEMO NUMBER — replace it in Admin → Payment methods. Use \"Send Money\" and write your booking code in the reference."},
		{"nagad", "01800000000", "personal", "DEMO NUMBER — replace it in Admin → Payment methods. Use \"Send Money\" and write your booking code in the reference."},
	}
	for i, m := range methods {
		err := db.WithContext(ctx).Exec(`INSERT INTO payment_methods (id, provider, account_name, account_number, account_type, instructions, is_active, position)
			VALUES (?, ?, 'CholoBangladesh (demo)', ?, ?, ?, TRUE, ?)`, uuid.Must(uuid.NewV7()), m.provider, m.number, m.kind, m.instructions, i).Error
		if err != nil {
			return err
		}
	}
	fmt.Println("✔ payment methods: demo bKash + Nagad numbers added (change them before going live)")
	return nil
}

type demoGig struct {
	guideEmail, spotSlug, title, description string
	price                                    int
	pricing                                  string
	hours                                    float64
	group                                    int
	langs, includes, excludes, days          []string
	meeting                                  string
}

var demoGigs = []demoGig{
	{"rahim.guide@example.com", "sundarbans", "Full-day Sundarbans boat safari with a licensed local guide",
		"We leave Mongla at 7 am by trawler, cruise the tidal creeks to Karamjal and Harbaria, walk the forest trails and look for spotted deer, crocodiles, monkeys and kingfishers. I handle the forest permits and safety briefing. Back at Mongla around 5 pm.",
		6000, "per_group", 10, 10, []string{"bn", "en", "hi"}, []string{"Boat ride", "Forest permit", "Lunch", "Bottled water"},
		[]string{"Hotel", "Transport to Mongla"}, []string{"fri", "sat", "sun"}, "Mongla ferry ghat"},
	{"nusrat.guide@example.com", "ratargul-swamp-forest", "Ratargul swamp forest boat tour from Sylhet",
		"A half-day trip from Sylhet city to Bangladesh's famous freshwater swamp forest. We take a small wooden boat between the submerged trees, climb the watch tower and stop for tea in a local village.",
		1200, "per_person", 5, 8, []string{"bn", "en"}, []string{"Wooden boat", "Entry ticket", "Tea and snacks"},
		[]string{"Transport from Sylhet"}, []string{}, "Motorghat, Gowainghat"},
	{"nusrat.guide@example.com", "jaflong", "Jaflong day trip: Piyain river, Khasi village & tea gardens",
		"Full day in Jaflong: boat on the crystal-clear Piyain river, the stone beds and zero point, a walk through a Khasi punji with betel-leaf gardens, and a stop at the Lalakhal tea estate on the way back.",
		4500, "per_group", 9, 8, []string{"bn", "en"}, []string{"Boat ride", "Lunch", "Guide fees"}, []string{"Car rental"},
		[]string{"sat", "sun", "mon", "tue", "wed", "thu", "fri"}, "Sylhet Zindabazar point"},
	{"nusrat.guide@example.com", "lawachara-national-park", "Lawachara rainforest trek & Sreemangal tea tasting",
		"A guided trek in Lawachara National Park to look for hoolock gibbons and birds, then the famous seven-layer tea and a walk through the tea gardens of Sreemangal.",
		900, "per_person", 6, 10, []string{"bn", "en"}, []string{"Park entry", "Seven-layer tea"}, []string{"Lunch"}, []string{}, "Sreemangal railway station"},
	{"arif.guide@example.com", "lalbagh-fort", "Old Dhaka heritage walk: Lalbagh Fort, Armenian Church & street food",
		"Start at the Mughal-era Lalbagh Fort, then walk the lanes of Old Dhaka to the Armenian Church and Star Mosque, and finish with Puran Dhaka's legendary biryani and lassi.",
		800, "per_person", 4, 12, []string{"bn", "en", "ar"}, []string{"Entry tickets", "Food tasting"}, []string{"Hotel pickup"},
		[]string{}, "Lalbagh Fort main gate"},
	{"arif.guide@example.com", "ahsan-manzil", "Ahsan Manzil & Buriganga river boat ride",
		"Visit the Pink Palace of the Nawabs of Dhaka with its museum galleries, then take a traditional boat across the busy Buriganga river from Sadarghat.",
		700, "per_person", 3, 12, []string{"bn", "en"}, []string{"Museum ticket", "Boat ride"}, []string{"Food"}, []string{"sat", "sun", "mon", "tue", "wed"}, "Ahsan Manzil gate"},
	{"arif.guide@example.com", "panam-nagar", "Sonargaon & Panam Nagar day trip from Dhaka",
		"Discover the ancient capital Sonargaon: the abandoned merchant houses of Panam Nagar, the Folk Art & Craft Museum and the Goaldi mosque. Round trip by car from Dhaka.",
		3500, "per_group", 7, 6, []string{"bn", "en"}, []string{"Car with driver", "Entry tickets"}, []string{"Lunch"}, []string{"fri", "sat"}, "Your hotel in Dhaka"},
}

func seedGigs(ctx context.Context, db *gorm.DB) error {
	created := 0
	for _, g := range demoGigs {
		var ids struct {
			GuideID uuid.UUID
			SpotID  uuid.UUID
		}
		err := db.WithContext(ctx).Raw(`SELECT u.id AS guide_id, s.id AS spot_id FROM users u, tourist_spots s
			WHERE u.email = ? AND u.role = 'guide' AND s.slug = ? AND s.deleted_at IS NULL`, g.guideEmail, g.spotSlug).Scan(&ids).Error
		if err != nil {
			return err
		}
		if ids.GuideID == uuid.Nil {
			continue
		}
		res := db.WithContext(ctx).Exec(`INSERT INTO guide_gigs (id, guide_id, spot_id, title, description, price, pricing_type,
			duration_hours, max_group_size, languages, includes, excludes, available_days, meeting_point, cancellation_policy, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'Free cancellation up to 24 hours before the tour.', 'active')
			ON CONFLICT DO NOTHING`, uuid.Must(uuid.NewV7()), ids.GuideID, ids.SpotID, g.title, g.description, g.price, g.pricing,
			g.hours, g.group, pq.StringArray(g.langs), pq.StringArray(g.includes), pq.StringArray(g.excludes),
			pq.StringArray(g.days), g.meeting)
		if res.Error != nil {
			return res.Error
		}
		created += int(res.RowsAffected)
	}
	if err := db.WithContext(ctx).Exec(`UPDATE tourist_spots s SET gig_count =
		(SELECT count(*) FROM guide_gigs g WHERE g.spot_id = s.id AND g.status = 'active' AND g.deleted_at IS NULL)`).Error; err != nil {
		return err
	}
	fmt.Printf("✔ demo gigs: %d created for %s\n", created, strings.Join([]string{"Rahim", "Nusrat", "Arif"}, ", "))
	return nil
}
