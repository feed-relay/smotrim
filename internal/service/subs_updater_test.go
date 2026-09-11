package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/internal/service/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (*SubsUpdater, *mocks.ClientMock, string) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "brands.json")
	subsFile := filepath.Join(tmpDir, "subscriptions.yml")

	clientMock := &mocks.ClientMock{}
	updater := &SubsUpdater{
		client:            clientMock,
		cacheFile:         cacheFile,
		subscriptionsFile: subsFile,
	}

	return updater, clientMock, tmpDir
}

func TestUpdateSubs_Success(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	brands := []graphql.Brand{
		{
			ID:     1,
			Title:  "Brand 1",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
		{
			ID:     2,
			Title:  "Brand 2 (Wrong Type)",
			Type:   &graphql.Type{Enum: "SomethingElse"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
		{
			ID:     3,
			Title:  "Brand 3 (Unpublished)",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Draft"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
		{
			ID:     4,
			Title:  "Brand 4 (Paid)",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PaidContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
		{
			ID:     5,
			Title:  "Brand 5 (No Channels)",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: nil,
		},
		{
			ID:     6,
			Title:  "Brand 6 (Podcast)",
			Type:   &graphql.Type{Enum: "Podcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 20, Title: "Channel 20"},
			},
		},
	}

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		res := &graphql.BrandsRaw{}
		res.Brands.Data = brands
		res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
			HasMorePages: false,
		}
		return res, nil
	}

	err := updater.UpdateSubs(ctx)
	require.NoError(t, err)

	content, err := os.ReadFile(updater.subscriptionsFile)
	require.NoError(t, err)

	res := string(content)
	assert.Contains(t, res, "subscriptions:")
	assert.Contains(t, res, "# Channel 10")
	assert.Contains(t, res, `"1" # Brand 1`)
	assert.NotContains(t, res, `"2"`)
	assert.NotContains(t, res, `"3"`)
	assert.NotContains(t, res, `"4"`)
	assert.NotContains(t, res, `"5"`)
	assert.Contains(t, res, "# Channel 20")
	assert.Contains(t, res, `"6" # Brand 6`)
}

func TestUpdateSubs_CacheHit(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	brands := []graphql.Brand{
		{
			ID:     1,
			Title:  "Brand 1",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
	}

	cache := brandsCache{
		UpdatedAt: time.Now(),
		Brands:    brands,
	}
	data, _ := json.Marshal(cache)
	err := os.WriteFile(updater.cacheFile, data, 0o600)
	require.NoError(t, err)

	err = updater.UpdateSubs(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, len(clientMock.BrandsRawCalls()), "Client.BrandsRaw should not be called on cache hit")

	content, err := os.ReadFile(updater.subscriptionsFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"1" # Brand 1`)
}

func TestUpdateSubs_CacheMiss(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	brands := []graphql.Brand{
		{
			ID:     1,
			Title:  "Brand 1",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
	}

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		res := &graphql.BrandsRaw{}
		res.Brands.Data = brands
		res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
			HasMorePages: false,
		}
		return res, nil
	}

	err := updater.UpdateSubs(ctx)
	require.NoError(t, err)

	assert.Greater(t, len(clientMock.BrandsRawCalls()), 0, "Client.BrandsRaw should be called on cache miss")

	// Verify cache was saved
	cacheData, err := os.ReadFile(updater.cacheFile)
	require.NoError(t, err)
	var cache brandsCache
	err = json.Unmarshal(cacheData, &cache)
	require.NoError(t, err)
	assert.Equal(t, 1, len(cache.Brands))
}

func TestUpdateSubs_CacheExpired(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	brands := []graphql.Brand{
		{
			ID:     1,
			Title:  "Brand 1",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
	}

	cache := brandsCache{
		UpdatedAt: time.Now().Add(-10 * 24 * time.Hour), // Expired
		Brands:    brands,
	}
	data, _ := json.Marshal(cache)
	err := os.WriteFile(updater.cacheFile, data, 0o600)
	require.NoError(t, err)

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		res := &graphql.BrandsRaw{}
		res.Brands.Data = brands
		res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
			HasMorePages: false,
		}
		return res, nil
	}

	err = updater.UpdateSubs(ctx)
	require.NoError(t, err)

	assert.Greater(t, len(clientMock.BrandsRawCalls()), 0, "Client.BrandsRaw should be called on expired cache")
}

func TestUpdateSubs_APIError(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		return nil, errors.New("api error")
	}

	err := updater.UpdateSubs(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get all brands: all brands: get brands page 1: api error")
}

func TestAllBrands_Pagination(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	page1 := []graphql.Brand{{ID: 1, Title: "B1"}}
	page2 := []graphql.Brand{{ID: 2, Title: "B2"}}

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		res := &graphql.BrandsRaw{}
		if page == 1 {
			res.Brands.Data = page1
			res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
				HasMorePages: true,
				LastPage:      2,
			}
			return res, nil
		}
		if page == 2 {
			res.Brands.Data = page2
			res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
				HasMorePages: false,
				LastPage:      2,
			}
			return res, nil
		}
		return nil, fmt.Errorf("unexpected page %d", page)
	}

	brands, err := updater.allBrands(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, len(brands))
	assert.Equal(t, 1, brands[0].ID)
	assert.Equal(t, 2, brands[1].ID)
}

func TestUpdateSubs_FileError(t *testing.T) {
	updater, clientMock, _ := setup(t)
	ctx := context.Background()

	brands := []graphql.Brand{
		{
			ID:     1,
			Title:  "Brand 1",
			Type:   &graphql.Type{Enum: "Radiobroadcast"},
			Status: &graphql.Status{Enum: "Published"},
			Tariff: &graphql.Tariff{Enum: "PublicContent"},
			Channels: []graphql.Channel{
				{ID: 10, Title: "Channel 10"},
			},
		},
	}

	clientMock.BrandsRawFunc = func(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error) {
		res := &graphql.BrandsRaw{}
		res.Brands.Data = brands
		res.Brands.PaginatorInfo = &graphql.PaginatorInfo{
			HasMorePages: false,
		}
		return res, nil
	}

	// Use an invalid path to trigger write error
	updater.subscriptionsFile = "/nonexistent/path/subs.yml"

	err := updater.UpdateSubs(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save subscriptions: write subscriptions file")
}
