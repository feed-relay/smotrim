package graphql

import (
	"context"
	"encoding/json"
	"fmt"
)

const brandsQuery = `query Brands(
	$first: Int = 10
	$page: Int = 1
) {
	brands(
		first: $first
		page: $page
	) {
		data {
			... on Brand {
				id
				title
				status {
					id
					enum
				}
				type {
					id
					enum
				}
				tariff {
					id
					enum
				}
				channels {
					... on Channel {
						id
						title
						slug
					}
				}
			}
		}
		paginatorInfo {
			currentPage
			lastPage
			hasMorePages
			lastItem
			total
			count
			perPage
			firstItem
		}
	}
}
`

type brandsVars struct {
	First int `json:"first"`
	Page  int `json:"page"`
}

type BrandsRaw struct {
	Brands struct {
		Data          []Brand        `json:"data,omitempty"`
		PaginatorInfo *PaginatorInfo `json:"paginatorInfo,omitempty"`
	} `json:"brands"`
}

func (c *Client) Brands(ctx context.Context, limit, page int) ([]Brand, error) {
	res, err := c.BrandsRaw(ctx, limit, page)
	if err != nil {
		return nil, err
	}
	return res.Brands.Data, nil
}

func (c *Client) BrandsRaw(ctx context.Context, limit, page int) (*BrandsRaw, error) {
	vars, err := json.Marshal(brandsVars{First: limit, Page: page})
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	var res BrandsRaw
	err = c.Do(ctx, "Brands", brandsQuery, string(vars), &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
