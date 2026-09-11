package graphql

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
)

//go:embed testdata/*
var testData embed.FS

type TestdataClient struct{}

func (c *TestdataClient) Channel(ctx context.Context, id int) (*Channel, error) {
	var res Channel
	err := c.do("testdata/channel.json", &res)
	return &res, err
}

func (c *TestdataClient) ChannelBySlug(ctx context.Context, slug string) (*Channel, error) {
	var res Channel
	err := c.do("testdata/channel.json", &res)
	return &res, err
}

func (c *TestdataClient) Brands(ctx context.Context, limit, page int) ([]Brand, error) {
	// res, err := c.BrandsRaw(ctx, limit, page)
	var res BrandsRaw
	err := c.do("testdata/brands.json", &res)
	return res.Brands.Data, err
}

func (c *TestdataClient) BrandsRaw(ctx context.Context, limit, page int) (*BrandsRaw, error) {
	var res BrandsRaw
	err := c.do("testdata/brands_raw.json", &res)
	return &res, err
}

func (c *TestdataClient) Brand(ctx context.Context, id int) (*Brand, error) {
	var res Brand
	err := c.do("testdata/brand.json", &res)
	return &res, err
}

func (c *TestdataClient) BrandEpisodes(ctx context.Context, id, limit int) (*BrandEpisodes, error) {
	var res brandEpisodesResult
	err := c.do("testdata/brand-episodes.json", &res)
	return &BrandEpisodes{
		Brand:    res.Brand,
		Episodes: res.EpisodesFilter.Data,
	}, err
}

func (c *TestdataClient) do(filename string, result any) error {
	file, err := testData.Open(filename)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("failed to close file", slog.Any("error", err))
		}
	}()

	if result != nil {
		if err := json.NewDecoder(file).Decode(result); err != nil {
			return fmt.Errorf("decode file: %w", err)
		}
	}

	return nil
}
