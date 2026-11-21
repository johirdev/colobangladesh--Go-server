// Command manage is the developer/ops CLI: seed demo data and reset state.
//
//	go run ./cmd/manage <command>                 (local, uses .env)
//	docker compose exec api manage <command>     (inside the container)
//
// Commands:
//
//	seed              create demo accounts (idempotent: existing ones are skipped)
//	reset-db          DROP every table, re-run migrations (dev only)
//	fresh             reset-db + reset-redis + seed (dev only)
//	reset-redis       FLUSHDB: OTPs, lockouts, rate limits, caches (dev only)
//	reset-ratelimit   clear only rate-limit counters (rl:*)
//	reset-locks       clear login/OTP lockouts, OTP cooldowns and hourly caps
//	unlock <id>       clear lockouts for one email / BD phone
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"cholobangladesh-server/internal/config"
	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/repository"
	"cholobangladesh-server/internal/platform/cache"
	"cholobangladesh-server/internal/platform/database"
	"cholobangladesh-server/pkg/password"
	"cholobangladesh-server/pkg/phone"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// SeedPassword is the password of every seeded account (development only).
const SeedPassword = "Password123"

const usage = `usage: manage <command> [--force]

  seed              create demo accounts (3 users, 3 guides, 1 admin)
  reset-db          drop all tables and re-run migrations       (dev only)
  fresh             reset-db + reset-redis + seed               (dev only)
  reset-redis       flush Redis (OTPs, locks, rate limits)      (dev only)
  reset-ratelimit   clear rate-limit counters only
  reset-locks       clear login/OTP lockouts and OTP cooldowns
  unlock <id>       clear lockouts for one email or BD phone

--force allows dev-only commands when APP_ENV=production (dangerous).`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	force := len(args) > 0 && args[len(args)-1] == "--force"
	devOnly := func() error {
		if cfg.App.IsProduction() && !force {
			return errors.New("refusing to run in production (APP_ENV=production); add --force if you really mean it")
		}
		return nil
	}
	ctx := context.Background()

	switch cmd {
	case "seed":
		if err := devOnly(); err != nil {
			return err
		}
		db, err := openDB(cfg)
		if err != nil {
			return err
		}
		return seed(ctx, db, cfg.Security.BcryptCost)

	case "reset-db":
		if err := devOnly(); err != nil {
			return err
		}
		db, err := openDB(cfg)
		if err != nil {
			return err
		}
		return resetDB(db)

	case "fresh":
		if err := devOnly(); err != nil {
			return err
		}
		db, err := openDB(cfg)
		if err != nil {
			return err
		}
		if err := resetDB(db); err != nil {
			return err
		}
		if err := withRedis(cfg, func(r *redis.Client) error { return r.FlushDB(ctx).Err() }); err != nil {
			return err
		}
		fmt.Println("✔ redis flushed")
		return seed(ctx, db, cfg.Security.BcryptCost)

	case "reset-redis":
		if err := devOnly(); err != nil {
			return err
		}
		return withRedis(cfg, func(r *redis.Client) error {
			if err := r.FlushDB(ctx).Err(); err != nil {
				return err
			}
			fmt.Println("✔ redis flushed (OTPs, lockouts, rate limits, caches)")
			return nil
		})

	case "reset-ratelimit":
		return withRedis(cfg, func(r *redis.Client) error {
			n, err := deletePattern(ctx, r, "rl:*")
			fmt.Printf("✔ %d rate-limit keys deleted\n", n)
			return err
		})

	case "reset-locks":
		return withRedis(cfg, func(r *redis.Client) error {
			total := 0
			for _, p := range []string{"cb:user:login:*", "cb:user:otp:block:*", "cb:user:otp:cd:*",
				"cb:user:otp:hr:*", "cb:user:sms:ip:*"} {
				n, err := deletePattern(ctx, r, p)
				if err != nil {
					return err
				}
				total += n
			}
			fmt.Printf("✔ %d lock/cooldown keys deleted\n", total)
			return nil
		})

	case "unlock":
		if len(args) == 0 || args[0] == "--force" {
			return errors.New("usage: manage unlock <email|phone>")
		}
		id := strings.ToLower(strings.TrimSpace(args[0]))
		if p, ok := phone.NormalizeBD(id); ok {
			id = p
		}
		return withRedis(cfg, func(r *redis.Client) error {
			n, err := r.Del(ctx, "cb:user:login:block:"+id, "cb:user:login:fail:"+id,
				"cb:user:otp:block:"+id, "cb:user:otp:hr:"+id).Result()
			fmt.Printf("✔ %s unlocked (%d keys)\n", id, n)
			return err
		})
	}
	fmt.Println(usage)
	return fmt.Errorf("unknown command %q", cmd)
}

func openDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := database.Connect(cfg.DB, false)
	if err != nil {
		return nil, err
	}
	return db, database.Migrate(db)
}

func withRedis(cfg *config.Config, fn func(*redis.Client) error) error {
	r, err := cache.Connect(cfg.Redis)
	if err != nil {
		return err
	}
	defer r.Close()
	return fn(r)
}

func deletePattern(ctx context.Context, r *redis.Client, pattern string) (int, error) {
	n := 0
	iter := r.Scan(ctx, 0, pattern, 500).Iterator()
	for iter.Next(ctx) {
		if err := r.Del(ctx, iter.Val()).Err(); err != nil {
			return n, err
		}
		n++
	}
	return n, iter.Err()
}

func resetDB(db *gorm.DB) error {
	if err := db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;").Error; err != nil {
		return err
	}
	fmt.Println("✔ all tables dropped")
	if err := database.Migrate(db); err != nil {
		return err
	}
	fmt.Println("✔ migrations re-applied")
	fmt.Println("! restart the API now so it drops cached DB connections:  docker compose restart api")
	return nil
}

// ------------------------------------------------------------------ seed data

type seedUser struct {
	Role     domain.Role
	FullName string
	Email    string
	Phone    string
	Country  string
	City     string
	Gender   string
	DOB      string
	Avatar   string
	Guide    *domain.GuideProfile
}

func s(v string) *string { return &v }
func i(v int) *int       { return &v }

func avatar(name string) string {
	return "https://ui-avatars.com/api/?size=300&background=0f766e&color=fff&name=" + strings.ReplaceAll(name, " ", "+")
}

var seedUsers = []seedUser{
	// ---- admin
	{Role: domain.RoleAdmin, FullName: "Super Admin", Email: "admin@cholobangladesh.com", Country: "BD", City: "Dhaka"},

	// ---- tourists (role = user)
	{Role: domain.RoleUser, FullName: "Emma Watson", Email: "emma@example.com", Country: "GB", City: "London",
		Gender: "female", DOB: "1995-04-21"},
	{Role: domain.RoleUser, FullName: "John Carter", Email: "john@example.com", Country: "US", City: "New York",
		Gender: "male", DOB: "1988-09-02"},
	{Role: domain.RoleUser, FullName: "Tanvir Ahmed", Email: "tanvir@example.com", Country: "BD", City: "Chattogram",
		Gender: "male", DOB: "1999-12-10"},

	// ---- guides (verified & listed)
	{Role: domain.RoleGuide, FullName: "Rahim Uddin", Phone: "01711000001", Email: "rahim.guide@example.com",
		Country: "BD", City: "Khulna", Gender: "male", DOB: "1987-03-15",
		Guide: &domain.GuideProfile{
			Headline:        s("Licensed Sundarbans mangrove & wildlife guide"),
			About:           s("Born near the Sundarbans, I have led more than 300 boat safaris to spot Royal Bengal tigers, crocodiles and rare birds. Safety first, and every trip is eco-friendly."),
			Languages:       pq.StringArray{"bn", "en", "hi"},
			ServiceAreas:    pq.StringArray{"khulna", "sundarbans", "bagerhat"},
			Specialties:     pq.StringArray{"nature_wildlife", "river_cruise", "photography"},
			ExperienceYears: 10, HourlyRate: i(900), DailyRate: i(6000), MaxGroupSize: 10,
			LicenseNumber: s("BTB-2016-0457"), RatingAvg: 4.9, RatingCount: 128, TotalTours: 310,
		}},
	{Role: domain.RoleGuide, FullName: "Nusrat Jahan", Phone: "01811000002", Email: "nusrat.guide@example.com",
		Country: "BD", City: "Sylhet", Gender: "female", DOB: "1993-07-08",
		Guide: &domain.GuideProfile{
			Headline:        s("Tea gardens, waterfalls & Sylheti food walks"),
			About:           s("I grew up among the tea estates of Sreemangal. I will take you to Ratargul swamp forest, Jaflong and the best local food spots that most tourists never find."),
			Languages:       pq.StringArray{"bn", "en"},
			ServiceAreas:    pq.StringArray{"sylhet", "sreemangal", "jaflong"},
			Specialties:     pq.StringArray{"tea_garden", "food", "nature_wildlife"},
			ExperienceYears: 6, HourlyRate: i(700), DailyRate: i(4500), MaxGroupSize: 8,
			RatingAvg: 4.8, RatingCount: 76, TotalTours: 140,
		}},
	{Role: domain.RoleGuide, FullName: "Arif Hossain", Phone: "01911000003", Email: "arif.guide@example.com",
		Country: "BD", City: "Dhaka", Gender: "male", DOB: "1990-11-25",
		Guide: &domain.GuideProfile{
			Headline:        s("Old Dhaka history, rickshaw rides & street food"),
			About:           s("Historian by training. Discover Lalbagh Fort, Ahsan Manzil, Sadarghat river port and Puran Dhaka's legendary biryani on a guided walk through 400 years of history."),
			Languages:       pq.StringArray{"bn", "en", "ar"},
			ServiceAreas:    pq.StringArray{"dhaka", "sonargaon"},
			Specialties:     pq.StringArray{"history_culture", "food", "city_tour", "religious_heritage"},
			ExperienceYears: 8, HourlyRate: i(600), DailyRate: i(4000), MaxGroupSize: 12,
			LicenseNumber: s("BTB-2019-1123"), RatingAvg: 4.7, RatingCount: 94, TotalTours: 220,
		}},
}

func seed(ctx context.Context, db *gorm.DB, cost int) error {
	repo := repository.NewPostgres(db)
	hasher := password.NewHasher(cost)
	hash, err := hasher.Hash(SeedPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	created, skipped := 0, 0
	for _, su := range seedUsers {
		email := strings.ToLower(su.Email)
		if exists, err := repo.EmailExists(ctx, email); err != nil {
			return err
		} else if exists {
			skipped++
			fmt.Printf("• skip   %-6s %s (already exists)\n", su.Role, email)
			continue
		}

		u := &domain.User{ID: uuid.Must(uuid.NewV7()), Role: su.Role, Status: domain.StatusActive,
			Email: &email, EmailVerifiedAt: &now, PasswordHash: &hash, TokenVersion: 1}
		if su.Phone != "" {
			p, _ := phone.NormalizeBD(su.Phone)
			u.Phone, u.PhoneVerifiedAt = &p, &now
		}
		lang := "en"
		if su.Country == "BD" {
			lang = "bn"
		}
		prof := &domain.UserProfile{UserID: u.ID, FullName: su.FullName, CountryCode: s(su.Country),
			City: s(su.City), PreferredLanguage: lang}
		if su.Gender != "" {
			prof.Gender = s(su.Gender)
		}
		if su.DOB != "" {
			d, _ := time.Parse("2006-01-02", su.DOB)
			prof.DateOfBirth = &d
		}
		if su.Role != domain.RoleAdmin {
			prof.AvatarURL = s(avatar(su.FullName))
		}
		u.Profile = prof

		if su.Guide != nil {
			g := *su.Guide
			g.UserID, g.Currency, g.IsAvailable = u.ID, "BDT", true
			g.VerificationStatus, g.VerifiedAt = domain.VerificationApproved, &now
			u.GuideProfile = &g
		}
		if err := repo.CreateUser(ctx, u); err != nil {
			return fmt.Errorf("seed %s: %w", email, err)
		}
		created++
		fmt.Printf("✔ create %-6s %-28s %s\n", su.Role, email, deref(u.Phone))
	}
	fmt.Printf("\nseed done: %d created, %d skipped. Password for every seeded account: %s\n", created, skipped, SeedPassword)
	return nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
