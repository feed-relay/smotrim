package graphql

import (
	"context"
	"encoding/json"
	"fmt"
)

const queryBrand = `query Brand($id: Int!) {
	brand(id: $id) {
		id
		title
		description
		channels {
			id
			title
			slug
		}
		images(linkTypes: [Poster, PosterCisq], presets: [Small]) {
			id
			linkType
			presets {
				name
				link
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
	}
}`

type brandResult struct {
	Data *Brand `json:"brand,omitempty"`
}

type brandVars struct {
	ID int `json:"id"`
}

// Brand fetches a brand by its numeric id
func (c *Client) Brand(ctx context.Context, id int) (*Brand, error) {
	vars, err := json.Marshal(brandVars{ID: id})
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	var res brandResult
	err = c.Do(ctx, "Brand", queryBrand, string(vars), &res)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}
