// Package dto holds the request/response contracts of the notification module.
package dto

import (
	"time"

	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
)

// ListQuery — ?page ?limit (20, max 50) ?q (title, text) ?sort=newest|oldest ?unread=true ?type=booking.accepted
type ListQuery struct {
	listing.Params
	Unread bool   `form:"unread" example:"false"`
	Type   string `form:"type" binding:"omitempty,max=40" example:"booking.accepted"`
}

// Item is the API shape of a notification.
type Item struct {
	ID        string         `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Type      string         `json:"type" example:"booking.accepted"`
	Title     string         `json:"title" example:"Booking accepted"`
	Body      string         `json:"body" example:"Rahim accepted your Sundarbans tour. Pay now to confirm."`
	Link      *string        `json:"link,omitempty" example:"/account/bookings/0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Data      map[string]any `json:"data,omitempty"`
	Read      bool           `json:"read" example:"false"`
	CreatedAt time.Time      `json:"created_at"`
}

// ItemPage documents response.Page[Item] for swagger.
type ItemPage struct {
	Items []Item            `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

type UnreadCount struct {
	Count int64 `json:"count" example:"3"`
}

type Ticket struct {
	Ticket    string `json:"ticket" example:"pX2...one-time-ticket"`
	ExpiresIn int    `json:"expires_in" example:"60"`
	Path      string `json:"path" example:"/socket.io/"`
}
