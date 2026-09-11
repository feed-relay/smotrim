package graphql

import (
	"encoding/json"
	"time"
)

type SmotrimTime time.Time

type Channel struct {
	ID    int    `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Slug  string `json:"slug,omitempty"`
}

type Brand struct {
	ID          int        `json:"id,omitempty"`
	Title       string     `json:"title,omitempty"`
	Description string     `json:"description,omitempty"`
	Status      *Status    `json:"status,omitempty"`
	Type        *Type      `json:"type,omitempty"`
	Tariff      *Tariff    `json:"tariff,omitempty"`
	Channels    []Channel  `json:"channels,omitempty"`
	Genres      []Genre    `json:"genres,omitempty"`
	Subgenres   []Subgenre `json:"subgenres,omitempty"`
	Images      []Image    `json:"images,omitempty"`
}

type Genre struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Subgenre struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Image struct {
	ID       int           `json:"id,omitempty"`
	LinkType string        `json:"linkType,omitempty"`
	Presets  []ImagePreset `json:"presets,omitempty"`
}

type ImagePreset struct {
	Name string `json:"name,omitempty"`
	Link string `json:"link,omitempty"`
}

type Episode struct {
	ID              int          `json:"id,omitempty"`
	CreatedAt       string       `json:"createdAt,omitempty"`
	Status          *Status      `json:"status,omitempty"`
	Title           string       `json:"title,omitempty"`
	Description     string       `json:"description,omitempty"`
	AirDate         *SmotrimTime `json:"airDate,omitempty"`
	PublicationDate string       `json:"publicationDate,omitempty"`
	Number          int          `json:"number,omitempty"`
	Season          *Season      `json:"season,omitempty"`
	Audio           *Audio       `json:"audio,omitempty"`
	Images          []Image      `json:"images,omitempty"`
}

type Audio struct {
	Duration int `json:"duration,omitempty"`
	PublicId int `json:"publicId,omitempty"`
}

type Season struct {
	Number int `json:"number,omitempty"`
}

type Status struct {
	ID   int    `json:"id,omitempty"`
	Enum string `json:"enum,omitempty"`
}

type Type struct {
	ID   int    `json:"id,omitempty"`
	Enum string `json:"enum,omitempty"`
	Code string `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

type Tariff struct {
	ID   int    `json:"id,omitempty"`
	Enum string `json:"enum,omitempty"`
}

type BrandEpisodes struct {
	Brand    *Brand
	Episodes []*Episode
}

type PaginatorInfo struct {
	CurrentPage  int  `json:"currentPage,omitempty"`
	PerPage      int  `json:"perPage,omitempty"`
	Total        int  `json:"total,omitempty"`
	Count        int  `json:"count,omitempty"`
	LastPage     int  `json:"lastPage,omitempty"`
	HasMorePages bool `json:"hasMorePages,omitempty"`
	FirstItem    int  `json:"firstItem,omitempty"`
	LastItem     int  `json:"lastItem,omitempty"`
}

func (t *SmotrimTime) UnmarshalJSON(data []byte) error {
	layouts := []string{
		"2006-01-02T15:04:05-0700",
		time.RFC3339,
	}
	if string(data) == "null" {
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, s)
		if err == nil {
			*t = SmotrimTime(parsed)
			return nil
		}
		lastErr = err
	}

	return lastErr
}

func (t SmotrimTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Time())
}

func (t *SmotrimTime) Time() time.Time {
	return time.Time(*t)
}
