package graphql

import (
	"context"
	"encoding/json"
	"fmt"
)

// if brand.type == Radiobroadcast - episodesFilter()
// TODO if brand.type == Podcast - brand.podcastMaterials()

const brandEpisodesQuery = `query BrandEpisodes(
	$brandId: Int!
	$page: Int = 1
	$first: Int!
	$airDateFrom: DateTime!
	$order: SortOrder = DESC
) {
	brand(id: $brandId) {
		id
		title
		description
		channels {
			id
			title
			description
			shortDescription
			slug
			images(linkTypes: [Logo, Icon], presets: [Small]) {
				... on Image {
					id
					linkType
					presets {
						link
					}
				}
			}
		}
		genres {
			id
			name
		}
		subgenres {
			id
			name
		}
		images(linkTypes: [Poster, PosterCisq], presets: [Small]) {
			id
			linkType
			presets {
				name
				link
			}
		}
	}
	episodesFilter(
		brand_id: $brandId
		first: $first
		page: $page
		airDateFrom: $airDateFrom
		orderBy: { column: EPISODES_AIR_DATE, order: $order }
	) {
		data {
			... on Episode {
			id
			title
			status {
				enum
			}
			number
			season {
				number
			}
			description
			createdAt
			airDate
			publicationDate
			audio {
				duration
				publicId
			}
			images(linkTypes: [SplashScreen], presets: [Small]) {
				id
				linkType
				presets {
					name
					link
				}
			}
		}
	}
}
}`

type brandEpisodesResult struct {
	Brand          *Brand `json:"brand"`
	EpisodesFilter struct {
		Data []*Episode `json:"data"`
	} `json:"episodesFilter"`
}

type brandEpisodesVars struct {
	BrandId     int    `json:"brandId"`
	Page        int    `json:"page"`
	First       int    `json:"first"`
	AirDateFrom string `json:"airDateFrom"`
	Order       string `json:"order"`
}

// BrandEpisodes fetches a brand episodes by brand's numeric id
func (c *Client) BrandEpisodes(ctx context.Context, id, limit int) (*BrandEpisodes, error) {
	vars, err := json.Marshal(brandEpisodesVars{
		BrandId: id,
		Page:    1,
		First:   limit,
		//AirDateFrom: "1900-01-01T00:00:00Z", // to avoid nullable episode.airDate
		AirDateFrom: "2026-07-01T00:00:00Z", // to avoid nullable episode.airDate
		Order:       "DESC",                 // by EPISODES_AIR_DATE
	})
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	var res brandEpisodesResult
	err = c.Do(ctx, "BrandEpisodes", brandEpisodesQuery, string(vars), &res)
	if err != nil {
		return nil, err
	}
	return &BrandEpisodes{
		Brand:    res.Brand,
		Episodes: res.EpisodesFilter.Data,
	}, nil
}
