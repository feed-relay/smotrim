package graphql

import (
	"context"
	"encoding/json"
	"fmt"
)

const queryChannel = `query Channel($id: Int!) {
	channel(id: $id) {
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
}`

const queryChannelBySlug = `query ChannelBySlug($slug: String!) {
	channel(slug: $slug) {
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
}`

type channelResult struct {
	Data *Channel `json:"channel,omitempty"`
}

type channelVars struct {
	ID int `json:"id"`
}

type channelBySlugVars struct {
	Slug string `json:"slug"`
}

// Channel fetches a brand by its numeric id
func (c *Client) Channel(ctx context.Context, id int) (*Channel, error) {
	vars, err := json.Marshal(channelVars{ID: id})
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	var res channelResult
	if err := c.Do(ctx, "Channel", queryChannel, string(vars), &res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

// ChannelBySlug fetches a brand by its slug
func (c *Client) ChannelBySlug(ctx context.Context, slug string) (*Channel, error) {
	vars, err := json.Marshal(channelBySlugVars{Slug: slug})
	if err != nil {
		return nil, fmt.Errorf("marshal variables: %w", err)
	}

	var res channelResult
	if err := c.Do(ctx, "ChannelBySlug", queryChannelBySlug, string(vars), &res); err != nil {
		return nil, err
	}
	return res.Data, nil
}
