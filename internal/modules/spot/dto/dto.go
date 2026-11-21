// Package dto holds the request/response contracts of the tourist spot module.
package dto

import (
	"time"

	"cholobangladesh-server/pkg/response"
)

// ================================================================== requests

type ImageInput struct {
	// ID keeps an existing image (and its uploaded file) when replacing the list. Omit for new URLs.
	ID        string  `json:"id" binding:"omitempty,uuid" example:""`
	URL       string  `json:"url" binding:"required,url,max=1000" example:"https://thumb.wikimedia.org/wikipedia/commons/thumb/c/cf/Cox%27s_Bazaar_Sunset_Sep2019.jpg/1280px-Cox%27s_Bazaar_Sunset_Sep2019.jpg"`
	Caption   *string `json:"caption" binding:"omitempty,max=300" example:"Sunset at Laboni Point"`
	SourceURL *string `json:"source_url" binding:"omitempty,url,max=500" example:"https://commons.wikimedia.org/wiki/File:Cox%27s_Bazaar_Sunset_Sep2019.jpg"`
}

type VideoInput struct {
	URL   string  `json:"url" binding:"required,url,max=300" example:"https://www.youtube.com/watch?v=y9vAaKi7SvU"`
	Title *string `json:"title" binding:"omitempty,max=300" example:"Cox's Bazar travel guide"`
}

// CreateSpotRequest — `upazila_ids`: 1-5 upazila/thana IDs, the first one is the primary
// location (its district/division are used for filters). A spot that lies in two thanas
// (e.g. Sundarbans: Mongla + Shyamnagar) lists both.
type CreateSpotRequest struct {
	Title           string       `json:"title" binding:"required,min=2,max=150" example:"Saint Martin's Island"`
	BnTitle         *string      `json:"bn_title" binding:"omitempty,max=150" example:"সেন্টমার্টিন দ্বীপ"`
	Slug            string       `json:"slug" binding:"omitempty,max=120,slug" example:"saint-martins-island"`
	Subtitle        *string      `json:"subtitle" binding:"omitempty,max=300" example:"Bangladesh's only coral island"`
	Category        string       `json:"category" binding:"required,oneof=beach island hill forest waterfall lake river wetland heritage religious park city other" example:"island"`
	LocationName    string       `json:"location_name" binding:"required,min=2,max=200" example:"Teknaf, Cox's Bazar"`
	Description     string       `json:"description" binding:"required,min=20,max=20000" example:"Saint Martin's Island is a small coral island..."`
	Highlights      []string     `json:"highlights" binding:"omitempty,max=20,dive,min=2,max=150" example:"Coral reefs,Chhera Dwip"`
	BestTimeToVisit *string      `json:"best_time_to_visit" binding:"omitempty,max=120" example:"December – February"`
	GoogleMapURL    *string      `json:"google_map_url" binding:"omitempty,url,max=500" example:"https://www.google.com/maps/search/?api=1&query=20.6237,92.3233"`
	Latitude        *float64     `json:"latitude" binding:"omitempty,gte=20,lte=27" example:"20.62375"`
	Longitude       *float64     `json:"longitude" binding:"omitempty,gte=88,lte=93" example:"92.3233"`
	UpazilaIDs      []int        `json:"upazila_ids" binding:"required,min=1,max=5,unique,dive,gte=1" example:"75"`
	Images          []ImageInput `json:"images" binding:"omitempty,max=50,dive"`
	Videos          []VideoInput `json:"videos" binding:"omitempty,max=20,dive"`
	SourceURL       *string      `json:"source_url" binding:"omitempty,url,max=500" example:"https://en.wikipedia.org/wiki/St._Martin%27s_Island"`
	Status          string       `json:"status" binding:"omitempty,oneof=draft published archived" example:"published"`
	IsFeatured      bool         `json:"is_featured" example:"true"`
}

// UpdateSpotRequest is a partial update. `upazila_ids`, `images` and `videos`, when sent,
// REPLACE the whole list (send existing image `id`s to keep uploaded files).
type UpdateSpotRequest struct {
	Title           *string       `json:"title" binding:"omitempty,min=2,max=150" example:"Saint Martin's Island"`
	BnTitle         *string       `json:"bn_title" binding:"omitempty,max=150" example:"সেন্টমার্টিন দ্বীপ"`
	Slug            *string       `json:"slug" binding:"omitempty,max=120,slug" example:"saint-martins-island"`
	Subtitle        *string       `json:"subtitle" binding:"omitempty,max=300" example:"Bangladesh's only coral island"`
	Category        *string       `json:"category" binding:"omitempty,oneof=beach island hill forest waterfall lake river wetland heritage religious park city other" example:"island"`
	LocationName    *string       `json:"location_name" binding:"omitempty,min=2,max=200" example:"Teknaf, Cox's Bazar"`
	Description     *string       `json:"description" binding:"omitempty,min=20,max=20000" example:"Saint Martin's Island is a small coral island..."`
	Highlights      *[]string     `json:"highlights" binding:"omitempty,max=20,dive,min=2,max=150" example:"Coral reefs,Chhera Dwip"`
	BestTimeToVisit *string       `json:"best_time_to_visit" binding:"omitempty,max=120" example:"December – February"`
	GoogleMapURL    *string       `json:"google_map_url" binding:"omitempty,url,max=500" example:"https://www.google.com/maps/search/?api=1&query=20.6237,92.3233"`
	Latitude        *float64      `json:"latitude" binding:"omitempty,gte=20,lte=27" example:"20.62375"`
	Longitude       *float64      `json:"longitude" binding:"omitempty,gte=88,lte=93" example:"92.3233"`
	UpazilaIDs      *[]int        `json:"upazila_ids" binding:"omitempty,min=1,max=5,unique,dive,gte=1" example:"75"`
	Images          *[]ImageInput `json:"images" binding:"omitempty,max=50,dive"`
	Videos          *[]VideoInput `json:"videos" binding:"omitempty,max=20,dive"`
	SourceURL       *string       `json:"source_url" binding:"omitempty,url,max=500"`
	Status          *string       `json:"status" binding:"omitempty,oneof=draft published archived" example:"published"`
	IsFeatured      *bool         `json:"is_featured" example:"false"`
}

type SpotQuery struct {
	DivisionID int    `form:"division_id" binding:"omitempty,gte=1" example:"1"`
	DistrictID int    `form:"district_id" binding:"omitempty,gte=1" example:"9"`
	UpazilaID  int    `form:"upazila_id" binding:"omitempty,gte=1" example:"75"`
	Category   string `form:"category" binding:"omitempty,oneof=beach island hill forest waterfall lake river wetland heritage religious park city other" example:"beach"`
	Featured   *bool  `form:"featured" example:"true"`
	Q          string `form:"q" binding:"omitempty,max=100" example:"beach"`
	Sort       string `form:"sort" binding:"omitempty,oneof=popular rating newest title" example:"popular"`
	Page       int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit      int    `form:"limit" binding:"omitempty,gte=1,lte=100" example:"20"`
}

// ManageSpotQuery adds the status filter (drafts/archived are visible to staff only).
type ManageSpotQuery struct {
	SpotQuery
	Status string `form:"status" binding:"omitempty,oneof=draft published archived" example:"draft"`
}

type MarkerQuery struct {
	DivisionID int `form:"division_id" binding:"omitempty,gte=1" example:"1"`
	DistrictID int `form:"district_id" binding:"omitempty,gte=1" example:"9"`
}

// AutofillRequest — type a spot name, get a pre-filled draft from Wikipedia (+ YouTube,
// Google image search and AI writing when their keys are configured). Nothing is saved.
type AutofillRequest struct {
	Name       string `json:"name" binding:"required,min=2,max=150" example:"Ratargul Swamp Forest"`
	DistrictID int    `json:"district_id" binding:"omitempty,gte=1" example:"36"`
}

// ================================================================== responses

type GeoRef struct {
	ID     int    `json:"id" example:"1"`
	Name   string `json:"name" example:"Chattogram"`
	BnName string `json:"bn_name" example:"চট্টগ্রাম"`
	Slug   string `json:"slug" example:"chattogram"`
}

type UpazilaRef struct {
	ID       int     `json:"id" example:"75"`
	Name     string  `json:"name" example:"Teknaf"`
	BnName   string  `json:"bn_name" example:"টেকনাফ"`
	Slug     string  `json:"slug" example:"teknaf"`
	Type     string  `json:"type" example:"upazila"`
	District *GeoRef `json:"district,omitempty"`
}

type ImageResponse struct {
	ID        string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	URL       string  `json:"url" example:"https://thumb.wikimedia.org/.../1280px-Saint_Martins_Island.jpg"`
	Caption   *string `json:"caption,omitempty" example:"Boats at the jetty"`
	SourceURL *string `json:"source_url,omitempty" example:"https://commons.wikimedia.org/wiki/File:Saint_Martins_Island.jpg"`
	Uploaded  bool    `json:"uploaded" example:"false"` // true = stored in our Cloudinary
}

type VideoResponse struct {
	ID           string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	YoutubeID    string  `json:"youtube_id" example:"y9vAaKi7SvU"`
	URL          string  `json:"url" example:"https://www.youtube.com/watch?v=y9vAaKi7SvU"`
	EmbedURL     string  `json:"embed_url" example:"https://www.youtube.com/embed/y9vAaKi7SvU"`
	ThumbnailURL string  `json:"thumbnail_url" example:"https://i.ytimg.com/vi/y9vAaKi7SvU/hqdefault.jpg"`
	Title        *string `json:"title,omitempty" example:"Saint Martin tour guide"`
}

// SpotCard is the compact shape used in grids, search results and the map hover card.
type SpotCard struct {
	ID            string   `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Slug          string   `json:"slug" example:"saint-martins-island"`
	Title         string   `json:"title" example:"Saint Martin's Island"`
	BnTitle       *string  `json:"bn_title,omitempty" example:"সেন্টমার্টিন দ্বীপ"`
	Subtitle      *string  `json:"subtitle,omitempty" example:"Bangladesh's only coral island"`
	Category      string   `json:"category" example:"island"`
	LocationName  string   `json:"location_name" example:"Teknaf, Cox's Bazar"`
	CoverImageURL *string  `json:"cover_image_url,omitempty"`
	Latitude      *float64 `json:"latitude" example:"20.62375"`
	Longitude     *float64 `json:"longitude" example:"92.3233"`
	Division      *GeoRef  `json:"division,omitempty"`
	District      *GeoRef  `json:"district,omitempty"`
	RatingAvg     float64  `json:"rating_avg" example:"4.8"`
	RatingCount   int      `json:"rating_count" example:"32"`
	GigCount      int      `json:"gig_count" example:"5"`
	IsFeatured    bool     `json:"is_featured" example:"true"`
	Status        string   `json:"status" example:"published"`
}

type SpotDetail struct {
	SpotCard
	Description     string          `json:"description"`
	Highlights      []string        `json:"highlights"`
	BestTimeToVisit *string         `json:"best_time_to_visit,omitempty" example:"December – February"`
	GoogleMapURL    *string         `json:"google_map_url,omitempty"`
	SourceURL       *string         `json:"source_url,omitempty"`
	Upazilas        []UpazilaRef    `json:"upazilas"`
	Images          []ImageResponse `json:"images"`
	Videos          []VideoResponse `json:"videos"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// SpotPage documents response.Page[SpotCard] for swagger.
type SpotPage struct {
	Items []SpotCard        `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

// MapMarker feeds the Bangladesh map (hover card: image, title, rating).
type MapMarker struct {
	ID            string   `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Slug          string   `json:"slug" example:"sajek-valley"`
	Title         string   `json:"title" example:"Sajek Valley"`
	BnTitle       *string  `json:"bn_title,omitempty" example:"সাজেক ভ্যালি"`
	Category      string   `json:"category" example:"hill"`
	CoverImageURL *string  `json:"cover_image_url,omitempty"`
	RatingAvg     float64  `json:"rating_avg" example:"4.7"`
	RatingCount   int      `json:"rating_count" example:"18"`
	GigCount      int      `json:"gig_count" example:"3"`
	Latitude      *float64 `json:"latitude" example:"23.635"`
	Longitude     *float64 `json:"longitude" example:"92.49"`
	DistrictID    int      `json:"district_id" example:"4"`
	DistrictSlug  string   `json:"district_slug" example:"rangamati"`
	DistrictName  string   `json:"district_name" example:"Rangamati"`
	DivisionID    int      `json:"division_id" example:"1"`
}

type OptionsResponse struct {
	Categories []string `json:"categories" example:"beach,island,hill"`
	Statuses   []string `json:"statuses" example:"draft,published,archived"`
	MaxImages  int      `json:"max_images" example:"50"`
	MaxVideos  int      `json:"max_videos" example:"20"`
}

// AutofillResponse is a suggested CreateSpotRequest; review it, then POST it to /manage/tourist-spots.
type AutofillResponse struct {
	Draft     CreateSpotRequest `json:"draft"`
	Suggested []UpazilaRef      `json:"suggested_upazilas"` // nearest upazilas to the coordinates
	Sources   []string          `json:"sources" example:"wikipedia,youtube,google_images,ai"`
	Notes     []string          `json:"notes" example:"YouTube: set YOUTUBE_API_KEY to fetch videos automatically"`
}
