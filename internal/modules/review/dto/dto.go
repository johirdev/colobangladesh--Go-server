// Package dto holds the request/response contracts of the review module.
package dto

import (
	"time"

	"cholobangladesh-server/pkg/response"
)

type ReviewQuery struct {
	GigID      string `form:"gig_id" binding:"omitempty,uuid" example:""`
	GuideID    string `form:"guide_id" binding:"omitempty,uuid" example:""`
	SpotID     string `form:"spot_id" binding:"omitempty,uuid" example:""`
	WithPhotos bool   `form:"with_photos" example:"false"`
	Sort       string `form:"sort" binding:"omitempty,oneof=newest highest lowest" example:"newest"`
	Page       int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit      int    `form:"limit" binding:"omitempty,gte=1,lte=50" example:"10"`
}

type ReplyRequest struct {
	Reply string `json:"reply" binding:"required,min=2,max=1000" example:"Thank you! It was a pleasure guiding your family."`
}

type VisibilityRequest struct {
	IsVisible *bool `json:"is_visible" binding:"required" example:"false"`
}

type Author struct {
	ID          string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	FullName    string  `json:"full_name" example:"Emma Watson"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	CountryCode *string `json:"country_code,omitempty" example:"GB"`
	FlagURL     *string `json:"flag_url,omitempty" example:"https://flagcdn.com/w40/gb.png"`
}

type ReviewResponse struct {
	ID         string     `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	BookingID  string     `json:"booking_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	GigID      string     `json:"gig_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	GigTitle   string     `json:"gig_title,omitempty" example:"Full-day Sundarbans boat safari"`
	GuideID    string     `json:"guide_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	SpotID     string     `json:"spot_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Rating     int        `json:"rating" example:"5"`
	Comment    string     `json:"comment" example:"Rahim was amazing — we saw a crocodile and spotted deer!"`
	Images     []string   `json:"images" example:"https://res.cloudinary.com/demo/image/upload/v1/cholobangladesh/reviews/x.jpg"`
	Author     Author     `json:"author"`
	GuideReply *string    `json:"guide_reply,omitempty"`
	RepliedAt  *time.Time `json:"replied_at,omitempty"`
	IsVisible  bool       `json:"is_visible" example:"true"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Summary struct {
	Average float64          `json:"average" example:"4.75"`
	Count   int64            `json:"count" example:"12"`
	Stars   map[string]int64 `json:"stars"` // "1".."5" -> count
}

type ReviewPage struct {
	Items []ReviewResponse  `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

type ReviewList struct {
	Summary Summary    `json:"summary"`
	Reviews ReviewPage `json:"reviews"`
}
