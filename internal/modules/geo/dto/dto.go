// Package dto holds the request/response contracts of the geo module.
package dto

import "cholobangladesh-server/pkg/response"

// ------------------------------------------------------------------ requests

// DivisionRequest creates a division. On PATCH every field is optional.
type DivisionRequest struct {
	Name      string   `json:"name" binding:"required,min=2,max=80" example:"Chattogram"`
	BnName    string   `json:"bn_name" binding:"required,min=1,max=80" example:"চট্টগ্রাম"`
	Slug      string   `json:"slug" binding:"omitempty,max=80,slug" example:"chattogram"`
	Latitude  *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"22.701308"`
	Longitude *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"91.705861"`
}

type DivisionPatch struct {
	Name      *string  `json:"name" binding:"omitempty,min=2,max=80" example:"Chattogram"`
	BnName    *string  `json:"bn_name" binding:"omitempty,min=1,max=80" example:"চট্টগ্রাম"`
	Slug      *string  `json:"slug" binding:"omitempty,max=80,slug" example:"chattogram"`
	Latitude  *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"22.701308"`
	Longitude *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"91.705861"`
}

type DistrictRequest struct {
	DivisionID int      `json:"division_id" binding:"required,gte=1" example:"1"`
	Name       string   `json:"name" binding:"required,min=2,max=80" example:"Cox's Bazar"`
	BnName     string   `json:"bn_name" binding:"required,min=1,max=80" example:"কক্সবাজার"`
	Slug       string   `json:"slug" binding:"omitempty,max=80,slug" example:"coxs-bazar"`
	Latitude   *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"21.44315"`
	Longitude  *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"91.97977"`
	Website    *string  `json:"website" binding:"omitempty,max=255" example:"www.coxsbazar.gov.bd"`
}

type DistrictPatch struct {
	DivisionID *int     `json:"division_id" binding:"omitempty,gte=1" example:"1"`
	Name       *string  `json:"name" binding:"omitempty,min=2,max=80" example:"Cox's Bazar"`
	BnName     *string  `json:"bn_name" binding:"omitempty,min=1,max=80" example:"কক্সবাজার"`
	Slug       *string  `json:"slug" binding:"omitempty,max=80,slug" example:"coxs-bazar"`
	Latitude   *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"21.44315"`
	Longitude  *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"91.97977"`
	Website    *string  `json:"website" binding:"omitempty,max=255" example:"www.coxsbazar.gov.bd"`
}

type UpazilaRequest struct {
	DistrictID int      `json:"district_id" binding:"required,gte=1" example:"9"`
	Name       string   `json:"name" binding:"required,min=2,max=100" example:"Teknaf"`
	BnName     string   `json:"bn_name" binding:"required,min=1,max=100" example:"টেকনাফ"`
	Slug       string   `json:"slug" binding:"omitempty,max=100,slug" example:"teknaf"`
	Type       string   `json:"type" binding:"omitempty,oneof=upazila metro_thana" example:"upazila"`
	Latitude   *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"20.8624"`
	Longitude  *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"92.3058"`
}

type UpazilaPatch struct {
	DistrictID *int     `json:"district_id" binding:"omitempty,gte=1" example:"9"`
	Name       *string  `json:"name" binding:"omitempty,min=2,max=100" example:"Teknaf"`
	BnName     *string  `json:"bn_name" binding:"omitempty,min=1,max=100" example:"টেকনাফ"`
	Slug       *string  `json:"slug" binding:"omitempty,max=100,slug" example:"teknaf"`
	Type       *string  `json:"type" binding:"omitempty,oneof=upazila metro_thana" example:"upazila"`
	Latitude   *float64 `json:"latitude" binding:"omitempty,gte=-90,lte=90" example:"20.8624"`
	Longitude  *float64 `json:"longitude" binding:"omitempty,gte=-180,lte=180" example:"92.3058"`
}

type DistrictQuery struct {
	DivisionID int    `form:"division_id" binding:"omitempty,gte=1" example:"1"`
	Q          string `form:"q" binding:"omitempty,max=80" example:"cox"`
}

type UpazilaQuery struct {
	DistrictID int    `form:"district_id" binding:"omitempty,gte=1" example:"9"`
	DivisionID int    `form:"division_id" binding:"omitempty,gte=1" example:"1"`
	Type       string `form:"type" binding:"omitempty,oneof=upazila metro_thana" example:"upazila"`
	Q          string `form:"q" binding:"omitempty,max=80" example:"tek"`
	Page       int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit      int    `form:"limit" binding:"omitempty,gte=1,lte=600" example:"100"`
}

// ------------------------------------------------------------------ responses

type DivisionResponse struct {
	ID            int      `json:"id" example:"1"`
	Name          string   `json:"name" example:"Chattogram"`
	BnName        string   `json:"bn_name" example:"চট্টগ্রাম"`
	Slug          string   `json:"slug" example:"chattogram"`
	Latitude      *float64 `json:"latitude" example:"22.701308"`
	Longitude     *float64 `json:"longitude" example:"91.705861"`
	DistrictCount int64    `json:"district_count" example:"11"`
}

type DivisionRef struct {
	ID     int    `json:"id" example:"1"`
	Name   string `json:"name" example:"Chattogram"`
	BnName string `json:"bn_name" example:"চট্টগ্রাম"`
	Slug   string `json:"slug" example:"chattogram"`
}

type DistrictResponse struct {
	ID           int          `json:"id" example:"9"`
	DivisionID   int          `json:"division_id" example:"1"`
	Division     *DivisionRef `json:"division,omitempty"`
	Name         string       `json:"name" example:"Cox's Bazar"`
	BnName       string       `json:"bn_name" example:"কক্সবাজার"`
	Slug         string       `json:"slug" example:"coxs-bazar"`
	Latitude     *float64     `json:"latitude" example:"21.44315"`
	Longitude    *float64     `json:"longitude" example:"91.97977"`
	Website      *string      `json:"website,omitempty" example:"www.coxsbazar.gov.bd"`
	UpazilaCount int64        `json:"upazila_count" example:"9"`
	SpotCount    int64        `json:"spot_count" example:"2"`
}

type DistrictRef struct {
	ID         int    `json:"id" example:"9"`
	DivisionID int    `json:"division_id" example:"1"`
	Name       string `json:"name" example:"Cox's Bazar"`
	BnName     string `json:"bn_name" example:"কক্সবাজার"`
	Slug       string `json:"slug" example:"coxs-bazar"`
}

type UpazilaResponse struct {
	ID         int          `json:"id" example:"75"`
	DistrictID int          `json:"district_id" example:"9"`
	District   *DistrictRef `json:"district,omitempty"`
	Name       string       `json:"name" example:"Teknaf"`
	BnName     string       `json:"bn_name" example:"টেকনাফ"`
	Slug       string       `json:"slug" example:"teknaf"`
	Type       string       `json:"type" example:"upazila" enums:"upazila,metro_thana"`
	Latitude   *float64     `json:"latitude" example:"20.8624"`
	Longitude  *float64     `json:"longitude" example:"92.3058"`
}

// UpazilaPage documents response.Page[UpazilaResponse] for swagger.
type UpazilaPage struct {
	Items []UpazilaResponse `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

// TreeDivision is the full hierarchy for cascading dropdowns (division -> district -> upazila).
type TreeDivision struct {
	ID        int            `json:"id" example:"1"`
	Name      string         `json:"name" example:"Chattogram"`
	BnName    string         `json:"bn_name" example:"চট্টগ্রাম"`
	Slug      string         `json:"slug" example:"chattogram"`
	Districts []TreeDistrict `json:"districts"`
}

type TreeDistrict struct {
	ID       int           `json:"id" example:"9"`
	Name     string        `json:"name" example:"Cox's Bazar"`
	BnName   string        `json:"bn_name" example:"কক্সবাজার"`
	Slug     string        `json:"slug" example:"coxs-bazar"`
	Upazilas []TreeUpazila `json:"upazilas"`
}

type TreeUpazila struct {
	ID     int    `json:"id" example:"75"`
	Name   string `json:"name" example:"Teknaf"`
	BnName string `json:"bn_name" example:"টেকনাফ"`
	Slug   string `json:"slug" example:"teknaf"`
	Type   string `json:"type" example:"upazila"`
}
