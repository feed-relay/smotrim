package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
)

//go:generate moq --out ./mocks/config_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Config
//go:generate moq --out ./mocks/client_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Client

const cacheTTL = 7 * 24 * time.Hour

const cacheFile = "var/cache/brands.json"

const subscriptionsFile = "etc/subscriptions.smotrim.yml"

type Config interface {
	TestData() bool
	HTTPTimeout() time.Duration
}

type Client interface {
	BrandsRaw(ctx context.Context, limit, page int) (*graphql.BrandsRaw, error)
}

type brandsCache struct {
	UpdatedAt time.Time       `json:"updatedAt"`
	Brands    []graphql.Brand `json:"brands"`
}

type channelBrands struct {
	Channel *graphql.Channel
	Brands  []graphql.Brand
}

type SubsUpdater struct {
	client            Client
	cacheFile         string
	subscriptionsFile string
}

func NewSubsUpdater(config Config) *SubsUpdater {
	var client Client
	if config.TestData() {
		client = api.NewTestdataClient()
	} else {
		client = api.NewClient(config.HTTPTimeout())
	}
	return &SubsUpdater{
		client:            client,
		cacheFile:         cacheFile,
		subscriptionsFile: subscriptionsFile,
	}
}

func (s *SubsUpdater) UpdateSubs(ctx context.Context) error {
	all, err := s.getAllBrands(ctx)
	if err != nil {
		return fmt.Errorf("get all brands: %w", err)
	}

	brandsByChannels := make(map[int]channelBrands)
	for _, b := range all {
		if b.Type.Enum != "Radiobroadcast" && b.Type.Enum != "Podcast" {
			continue
		}
		if b.Status.Enum != "Published" {
			continue
		}
		if b.Tariff.Enum != "PublicContent" {
			continue
		}
		if b.Channels == nil || len(b.Channels) == 0 {
			continue
		}

		for _, c := range b.Channels {
			cb := brandsByChannels[c.ID]
			cb.Channel = &c
			cb.Brands = append(cb.Brands, b)
			brandsByChannels[c.ID] = cb
		}
	}

	err = s.saveSubscriptions(s.subscriptionsFile, brandsByChannels, 10)
	if err != nil {
		return fmt.Errorf("save subscriptions: %w", err)
	}

	return nil
}

func (s *SubsUpdater) getAllBrands(ctx context.Context) ([]graphql.Brand, error) {
	brands, ok, err := s.loadBrandsCache(s.cacheFile)
	if err != nil {
		return nil, fmt.Errorf("load brands cache: %w", err)
	}
	if ok {
		return brands, nil
	}

	brands, err = s.allBrands(ctx)
	if err != nil {
		return nil, fmt.Errorf("all brands: %w", err)
	}

	if err = s.saveBrandsCache(s.cacheFile, brands); err != nil {
		return nil, fmt.Errorf("save brands cache: %w", err)
	}

	return brands, nil
}

func (s *SubsUpdater) allBrands(ctx context.Context) ([]graphql.Brand, error) {
	limit := 100

	var all []graphql.Brand

	for page := 1; ; page++ {
		result, err := s.client.BrandsRaw(ctx, limit, page)
		if err != nil {
			return nil, fmt.Errorf("get brands page %d: %w", page, err)
		}

		all = append(all, result.Brands.Data...)

		p := result.Brands.PaginatorInfo
		if p == nil || !p.HasMorePages {
			break
		}
		if p.LastPage > 0 && page >= p.LastPage {
			break
		}
	}

	return all, nil
}

func (s *SubsUpdater) loadBrandsCache(filename string) ([]graphql.Brand, bool, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// no cache file
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read cache file: %w", err)
	}

	var cache brandsCache
	if err = json.Unmarshal(data, &cache); err != nil {
		return nil, false, fmt.Errorf("unmarshal cache: %w", err)
	}

	if time.Since(cache.UpdatedAt) >= cacheTTL {

		return nil, false, nil
	}

	return cache.Brands, true, nil
}

func (s *SubsUpdater) saveBrandsCache(filename string, brands []graphql.Brand) error {
	cache := brandsCache{
		UpdatedAt: time.Now(),
		Brands:    brands,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir cache dir: %w", err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	return nil
}

func (s *SubsUpdater) saveSubscriptions(filename string, brandsByChannels map[int]channelBrands, limit int) error {
	var stringBuilder strings.Builder

	stringBuilder.WriteString("subscriptions:\n")

	for _, cb := range brandsByChannels {
		bb := cb.Brands
		if len(bb) == 0 {
			continue
		}

		fmt.Fprintf(&stringBuilder, "  # %s\n", cb.Channel.Title)
		stringBuilder.WriteString("  - platform: smotrim\n")
		fmt.Fprintf(&stringBuilder, "    limit: %d\n", limit)
		stringBuilder.WriteString("    shows:\n")

		for _, b := range bb {
			fmt.Fprintf(
				&stringBuilder,
				"      - %q # %s\n",
				strconv.Itoa(b.ID),
				b.Title,
			)
		}

		stringBuilder.WriteString("\n")
	}

	if err := os.WriteFile(filename, []byte(stringBuilder.String()), 0644); err != nil {
		return fmt.Errorf("write subscriptions file: %w", err)
	}
	return nil
}
