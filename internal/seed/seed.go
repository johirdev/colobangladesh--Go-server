// Package seed loads default/reference data that ships with the binary:
//
//   - geo.json            8 divisions, 64 districts, 495 upazilas + Dhaka/Chattogram metro thanas
//     (source: github.com/nuhil/bangladesh-geocode, Bangla names + lat/lng)
//   - tourist_spots.json  20 famous tourist spots (text, coordinates and images from Wikipedia /
//     Wikimedia Commons, YouTube videos), published by default and editable by admins/moderators
//
// Every function is idempotent: existing rows (matched by id or slug) are never overwritten,
// so admin edits survive re-seeding. Deleted default spots are not re-created either.
package seed

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed data/geo.json
var geoJSON []byte

//go:embed data/tourist_spots.json
var spotsJSON []byte

// ------------------------------------------------------------------ geo

type geoFile struct {
	Divisions []struct {
		ID     int      `json:"id"`
		Name   string   `json:"name"`
		BnName string   `json:"bn_name"`
		Slug   string   `json:"slug"`
		Lat    *float64 `json:"lat"`
		Lng    *float64 `json:"lng"`
	} `json:"divisions"`
	Districts []struct {
		ID         int      `json:"id"`
		DivisionID int      `json:"division_id"`
		Name       string   `json:"name"`
		BnName     string   `json:"bn_name"`
		Slug       string   `json:"slug"`
		Lat        *float64 `json:"lat"`
		Lng        *float64 `json:"lng"`
		Website    *string  `json:"website"`
	} `json:"districts"`
	Upazilas []struct {
		ID         int      `json:"id"`
		DistrictID int      `json:"district_id"`
		Name       string   `json:"name"`
		BnName     string   `json:"bn_name"`
		Slug       string   `json:"slug"`
		Type       string   `json:"type"`
		Lat        *float64 `json:"lat"`
		Lng        *float64 `json:"lng"`
	} `json:"upazilas"`
}

type divisionRow struct {
	ID                  int
	Name, BnName, Slug  string
	Latitude, Longitude *float64
}

func (divisionRow) TableName() string { return "divisions" }

type districtRow struct {
	ID, DivisionID      int
	Name, BnName, Slug  string
	Latitude, Longitude *float64
	Website             *string
}

func (districtRow) TableName() string { return "districts" }

type upazilaRow struct {
	ID, DistrictID           int
	Name, BnName, Slug, Type string
	Latitude, Longitude      *float64
}

func (upazilaRow) TableName() string { return "upazilas" }

// GeoCounts reports how many rows each table has after seeding.
type GeoCounts struct{ Divisions, Districts, Upazilas int64 }

// Geo inserts missing divisions, districts and upazilas (ON CONFLICT DO NOTHING).
func Geo(ctx context.Context, db *gorm.DB) (GeoCounts, error) {
	var f geoFile
	if err := json.Unmarshal(geoJSON, &f); err != nil {
		return GeoCounts{}, fmt.Errorf("seed geo: %w", err)
	}
	var c GeoCounts
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		skip := clause.OnConflict{DoNothing: true}
		divs := make([]divisionRow, 0, len(f.Divisions))
		for _, d := range f.Divisions {
			divs = append(divs, divisionRow{d.ID, d.Name, d.BnName, d.Slug, d.Lat, d.Lng})
		}
		if err := tx.Clauses(skip).Create(&divs).Error; err != nil {
			return err
		}
		dists := make([]districtRow, 0, len(f.Districts))
		for _, d := range f.Districts {
			dists = append(dists, districtRow{d.ID, d.DivisionID, d.Name, d.BnName, d.Slug, d.Lat, d.Lng, d.Website})
		}
		if err := tx.Clauses(skip).Create(&dists).Error; err != nil {
			return err
		}
		ups := make([]upazilaRow, 0, len(f.Upazilas))
		for _, u := range f.Upazilas {
			ups = append(ups, upazilaRow{u.ID, u.DistrictID, u.Name, u.BnName, u.Slug, u.Type, u.Lat, u.Lng})
		}
		if err := tx.Clauses(skip).CreateInBatches(&ups, 200).Error; err != nil {
			return err
		}
		// Explicit IDs were inserted, so move each SERIAL past the highest ID.
		for _, t := range []string{"divisions", "districts", "upazilas"} {
			if err := tx.Exec(fmt.Sprintf(
				"SELECT setval(pg_get_serial_sequence('%s', 'id'), GREATEST((SELECT COALESCE(MAX(id), 0) FROM %s), 1))", t, t),
			).Error; err != nil {
				return err
			}
		}
		tx.Table("divisions").Count(&c.Divisions)
		tx.Table("districts").Count(&c.Districts)
		tx.Table("upazilas").Count(&c.Upazilas)
		return nil
	})
	return c, err
}

// GeoIsEmpty reports whether reference data still has to be loaded.
func GeoIsEmpty(ctx context.Context, db *gorm.DB) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Table("divisions").Count(&n).Error
	return n == 0, err
}

// ------------------------------------------------------------------ tourist spots

// Spot is one entry of tourist_spots.json.
type Spot struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	BnTitle     string   `json:"bn_title"`
	Subtitle    string   `json:"subtitle"`
	Category    string   `json:"category"`
	Location    string   `json:"location_name"`
	Description string   `json:"description"`
	Highlights  []string `json:"highlights"`
	BestTime    string   `json:"best_time_to_visit"`
	Latitude    float64  `json:"latitude"`
	Longitude   float64  `json:"longitude"`
	MapURL      string   `json:"google_map_url"`
	Featured    bool     `json:"is_featured"`
	SourceURL   string   `json:"source_url"`
	Upazilas    []struct {
		District string `json:"district"`
		Upazila  string `json:"upazila"`
	} `json:"upazilas"`
	Images []struct {
		URL       string `json:"url"`
		Caption   string `json:"caption"`
		SourceURL string `json:"source_url"`
	} `json:"images"`
	Videos []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	} `json:"videos"`
}

// DefaultSpots returns the bundled tourist spots.
func DefaultSpots() ([]Spot, error) {
	var s []Spot
	if err := json.Unmarshal(spotsJSON, &s); err != nil {
		return nil, fmt.Errorf("seed spots: %w", err)
	}
	return s, nil
}

type spotRow struct {
	ID              uuid.UUID
	Slug            string
	Title           string
	BnTitle         *string
	Subtitle        *string
	Category        string
	LocationName    string
	Description     string
	Highlights      pq.StringArray `gorm:"type:text[]"`
	BestTimeToVisit *string
	GoogleMapURL    *string `gorm:"column:google_map_url"`
	Latitude        *float64
	Longitude       *float64
	DivisionID      int
	DistrictID      int
	CoverImageURL   *string `gorm:"column:cover_image_url"`
	SourceURL       *string `gorm:"column:source_url"`
	Status          string
	IsFeatured      bool
	CreatedBy       *uuid.UUID
	UpdatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (spotRow) TableName() string { return "tourist_spots" }

type spotUpazilaRow struct {
	SpotID    uuid.UUID
	UpazilaID int
	Position  int
}

func (spotUpazilaRow) TableName() string { return "tourist_spot_upazilas" }

type spotImageRow struct {
	ID        uuid.UUID
	SpotID    uuid.UUID
	URL       string `gorm:"column:url"`
	Caption   *string
	SourceURL *string `gorm:"column:source_url"`
	Position  int
	CreatedAt time.Time
}

func (spotImageRow) TableName() string { return "tourist_spot_images" }

type spotVideoRow struct {
	ID        uuid.UUID
	SpotID    uuid.UUID
	YoutubeID string
	URL       string `gorm:"column:url"`
	Title     *string
	Position  int
	CreatedAt time.Time
}

func (spotVideoRow) TableName() string { return "tourist_spot_videos" }

func strPtr(s string) *string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &s
}

// YouTubeID extracts the 11-character video ID from common YouTube URL formats.
func YouTubeID(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, marker := range []string{"v=", "youtu.be/", "/embed/", "/shorts/", "/live/"} {
		if i := strings.Index(raw, marker); i >= 0 {
			id := raw[i+len(marker):]
			if j := strings.IndexAny(id, "?&#/"); j >= 0 {
				id = id[:j]
			}
			if len(id) == 11 {
				return id
			}
		}
	}
	return ""
}

// Spots inserts the default tourist spots that do not exist yet (matched by slug,
// including soft-deleted ones). Returns (created, skipped).
func Spots(ctx context.Context, db *gorm.DB, actor *uuid.UUID) (int, int, error) {
	spots, err := DefaultSpots()
	if err != nil {
		return 0, 0, err
	}
	created, skipped := 0, 0
	for _, s := range spots {
		var n int64
		if err := db.WithContext(ctx).Table("tourist_spots").Where("slug = ?", s.Slug).Count(&n).Error; err != nil {
			return created, skipped, err
		}
		if n > 0 {
			skipped++
			continue
		}
		if err := insertSpot(ctx, db, s, actor); err != nil {
			return created, skipped, fmt.Errorf("seed spot %s: %w", s.Slug, err)
		}
		created++
	}
	return created, skipped, nil
}

func insertSpot(ctx context.Context, db *gorm.DB, s Spot, actor *uuid.UUID) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		type loc struct{ UpazilaID, DistrictID, DivisionID int }
		locs := make([]loc, 0, len(s.Upazilas))
		for _, u := range s.Upazilas {
			var l loc
			err := tx.Raw(`SELECT u.id AS upazila_id, d.id AS district_id, d.division_id
				FROM upazilas u JOIN districts d ON d.id = u.district_id
				WHERE d.slug = ? AND u.slug = ?`, u.District, u.Upazila).Scan(&l).Error
			if err != nil {
				return err
			}
			if l.UpazilaID == 0 {
				return fmt.Errorf("upazila %s/%s not found (seed geo first)", u.District, u.Upazila)
			}
			locs = append(locs, l)
		}
		if len(locs) == 0 {
			return fmt.Errorf("no upazila")
		}
		now := time.Now().UTC()
		lat, lng := s.Latitude, s.Longitude
		row := spotRow{
			ID: uuid.Must(uuid.NewV7()), Slug: s.Slug, Title: s.Title, BnTitle: strPtr(s.BnTitle), Subtitle: strPtr(s.Subtitle),
			Category: s.Category, LocationName: s.Location, Description: s.Description, Highlights: s.Highlights,
			BestTimeToVisit: strPtr(s.BestTime), GoogleMapURL: strPtr(s.MapURL), Latitude: &lat, Longitude: &lng,
			DivisionID: locs[0].DivisionID, DistrictID: locs[0].DistrictID, SourceURL: strPtr(s.SourceURL),
			Status: "published", IsFeatured: s.Featured, CreatedBy: actor, UpdatedBy: actor, CreatedAt: now, UpdatedAt: now,
		}
		if len(s.Images) > 0 {
			row.CoverImageURL = &s.Images[0].URL
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for i, l := range locs {
			if err := tx.Create(&spotUpazilaRow{SpotID: row.ID, UpazilaID: l.UpazilaID, Position: i}).Error; err != nil {
				return err
			}
		}
		for i, im := range s.Images {
			r := spotImageRow{ID: uuid.Must(uuid.NewV7()), SpotID: row.ID, URL: im.URL, Caption: strPtr(im.Caption),
				SourceURL: strPtr(im.SourceURL), Position: i, CreatedAt: now}
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		for i, v := range s.Videos {
			id := YouTubeID(v.URL)
			if id == "" {
				continue
			}
			r := spotVideoRow{ID: uuid.Must(uuid.NewV7()), SpotID: row.ID, YoutubeID: id, URL: v.URL, Title: strPtr(v.Title),
				Position: i, CreatedAt: now}
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
