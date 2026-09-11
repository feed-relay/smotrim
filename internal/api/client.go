package api

import (
	"context"
	"net/http"
	"time"

	"github.com/feed-relay/smotrim/internal/api/graphql"
)

const defaultHTTPTimeout = 60 * time.Second

type api interface {
	Audio(ctx context.Context, publicId int) (*Audio, error)
}

type gql interface {
	BrandEpisodes(ctx context.Context, id int, limit int) (*graphql.BrandEpisodes, error)
	BrandsRaw(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error)
}

type Client struct {
	api api
	gql gql
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	httpClient := &http.Client{Timeout: timeout}

	return &Client{
		api: &apiClient{HTTPClient: httpClient},
		gql: &graphql.Client{HTTPClient: httpClient},
	}
}

func NewTestdataClient() *Client {
	return &Client{
		api: &testdataApiClient{},
		gql: &graphql.TestdataClient{},
	}
}

func (c *Client) BrandsRaw(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
	return c.gql.BrandsRaw(ctx, limit, page)
}

func (c *Client) BrandEpisodes(ctx context.Context, id, limit int) (*graphql.BrandEpisodes, error) {
	return c.gql.BrandEpisodes(ctx, id, limit)
}

func (c *Client) Audio(ctx context.Context, publicId int) (*Audio, error) {
	return c.api.Audio(ctx, publicId)
}
