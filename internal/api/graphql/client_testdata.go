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

func (c *TestdataClient) Channel(_ context.Context, _ int) (*Channel, error) {
	var res Channel
	err := c.do("testdata/channel.json", &res)
	return &res, err
}

func (c *TestdataClient) ChannelBySlug(_ context.Context, _ string) (*Channel, error) {
	var res Channel
	err := c.do("testdata/channel.json", &res)
	return &res, err
}

func (c *TestdataClient) Brands(_ context.Context, _, _ int) ([]Brand, error) {
	// res, err := c.BrandsRaw(ctx, limit, page)
	var res BrandsRaw
	err := c.do("testdata/brands.json", &res)
	return res.Brands.Data, err
}

func (c *TestdataClient) BrandsRaw(_ context.Context, _, _ int) (*BrandsRaw, error) {
	var res BrandsRaw
	err := c.do("testdata/brands_raw.json", &res)
	return &res, err
}

func (c *TestdataClient) Brand(_ context.Context, _ int) (*Brand, error) {
	var res Brand
	err := c.do("testdata/brand.json", &res)
	return &res, err
}

func (c *TestdataClient) BrandEpisodes(_ context.Context, _, _ int) (*BrandEpisodes, error) {
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
