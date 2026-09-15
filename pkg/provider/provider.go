package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/feed-relay/contracts"
	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/internal/media"
)

//go:generate moq --out ./mocks/config_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Config
//go:generate moq --out ./mocks/client_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Client
//go:generate moq --out ./mocks/adapter_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Adapter
//go:generate moq --out ./mocks/sub_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Sub

// Platform is this provider's identifier.
const Platform = "smotrim"

// feedWorkers is the maximum number of shows processed concurrently.
const feedWorkers = 4

// clientRequests is the maximum number of concurrent p.client calls
// (BrandEpisodes and Audio combined) across all in-flight shows.
const clientRequests = 8

// Provider Smotrim data.
//
// Should implement `contracts.Provider` interface
//
//	type Provider interface {
//		Platform() string
//		Feeds(ctx context.Context, subscriptions []Subscription) (map[string]*rsscast.Feed, error)
//	}
type Provider struct {
	config  Config
	client  Client
	adapter Adapter
}

// Client fetches brand/episode metadata and per-episode audio info from
// the Smotrim GraphQL and player APIs.
type Client interface {
	BrandEpisodes(ctx context.Context, id int, limit int) (*graphql.BrandEpisodes, error)
	Audio(ctx context.Context, publicId int) (*api.Audio, error)
}

// Adapter converts Smotrim domain models into an RSS feed.
type Adapter interface {
	Feed(ctx context.Context, channel *graphql.Channel, brand *graphql.Brand, episodes []*graphql.Episode, audios map[int]*api.Audio) (*rsscast.Feed, error)
}

type Sub interface {
	Limit() int
	Shows() []string
}

// feedTask is one (subscription, show) pair to be processed by a worker.
type feedTask struct {
	subscription Sub
	show         string
}

// NewProvider builds a Provider backed by config.
func NewProvider(config Config) *Provider {
	var client Client
	var sizer FileSizer

	if config.TestData() {
		client = api.NewTestdataClient()
		sizer = &media.EmptySizer{}
	} else {
		client = api.NewClient(config.HTTPTimeout(), clientRequests)
		sizer = media.NewHttpFileSizer(4)
	}

	return &Provider{
		config: config,
		client: client,
		adapter: &adapter{
			config:    config,
			fileSizer: sizer,
		},
	}
}

func (p *Provider) Platform() string {
	return Platform
}

// Feeds builds an RSS feed for every show across subscriptions, keyed by
// feed slug. Shows are processed concurrently (up to feedWorkers at a
// time); a failure building one show's feed doesn't stop the others. Any
// per-show errors are joined and always returned alongside whatever feeds
// did succeed - a partial failure is never silently dropped. The returned
// map is nil only if every show failed, and an error only if at least one
// show failed (or subscriptions has no shows at all).
func (p *Provider) Feeds(ctx context.Context, subscriptions []contracts.Subscription) (map[string]*rsscast.Feed, error) {
	var allTasks []feedTask
	for _, subscription := range subscriptions {
		for _, show := range subscription.Shows() {
			allTasks = append(allTasks, feedTask{subscription: subscription, show: show})
		}
	}

	workers := min(feedWorkers, len(allTasks))
	if workers == 0 {
		return nil, errors.New("smotrim: no shows")
	}

	tasks := make(chan feedTask)

	var wg sync.WaitGroup
	var mu sync.Mutex

	feeds := make(map[string]*rsscast.Feed)
	var errs []error

	// Limits the total number of concurrent requests to p.client,
	// including both BrandEpisodes and Audio calls.
	requests := make(chan struct{}, clientRequests)

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for task := range tasks {
				slug, feed, err := p.feed(ctx, task.subscription, task.show, requests)
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					continue
				}

				mu.Lock()
				feeds[slug] = feed
				mu.Unlock()
			}
		}()
	}

	for _, task := range allTasks {
		tasks <- task
	}

	close(tasks)
	wg.Wait()

	var err error
	if len(errs) > 0 {
		err = errors.Join(errs...)
	}

	if len(feeds) > 0 {
		// Return whatever succeeded alongside any errors, rather than
		// silently discarding the errors just because some shows worked.
		return feeds, err
	}

	return nil, err
}

// feed builds the RSS feed for a single show. requests gates p.client
// calls; feed acquires and releases a slot around its own BrandEpisodes
// call, respecting ctx cancellation while waiting for a slot.
func (p *Provider) feed(ctx context.Context, subscription Sub, show string, requests chan struct{}) (string, *rsscast.Feed, error) {
	brandID, err := p.extractNumber(show)
	if err != nil {
		return "", nil, fmt.Errorf("extract number failed: %w", err)
	}

	brandEpisodes, err := p.brandEpisodes(
		ctx,
		brandID,
		subscription.Limit(),
		requests,
	)

	if err != nil {
		return "", nil, fmt.Errorf("fetch brand episodes failed: %w", err)
	}

	if brandEpisodes == nil || brandEpisodes.Brand == nil {
		return "", nil, fmt.Errorf("smotrim: brand %d not found", brandID)
	}

	if len(brandEpisodes.Brand.Channels) == 0 {
		return "", nil, fmt.Errorf(
			"smotrim: brand %d has no channels",
			brandID,
		)
	}

	audioData, err := p.audioData(ctx, brandEpisodes.Episodes, requests)
	if err != nil {
		slog.Error("smotrim: audio data err", "err", err)
	}

	feed, err := p.adapter.Feed(
		ctx,
		&brandEpisodes.Brand.Channels[0],
		brandEpisodes.Brand,
		brandEpisodes.Episodes,
		audioData,
	)
	if err != nil {
		return "", feed, fmt.Errorf("build feed failed: %w", err)
	}

	slug := fmt.Sprintf("%s-brand-%d", p.Platform(), brandID)

	return slug, feed, nil
}

// audioData fetches Audio for every episode that has one linked
// (episode.Audio != nil), keyed by audio public ID, concurrently.
//
// Successful results are returned even when one or more requests fail.
// All request errors are joined and returned as a single error.
func (p *Provider) audioData(ctx context.Context, episodes []*graphql.Episode, requests chan struct{}) (map[int]*api.Audio, error) {
	audioData := make(map[int]*api.Audio, len(episodes))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, episode := range episodes {
		if episode == nil || episode.Audio == nil {
			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			audio, err := p.audio(ctx, episode.Audio.PublicId, requests)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			audioData[episode.Audio.PublicId] = audio
			mu.Unlock()
		}()
	}

	wg.Wait()

	return audioData, errors.Join(errs...)
}

func (p *Provider) brandEpisodes(
	ctx context.Context,
	brandID int,
	limit int,
	requests chan struct{},
) (*graphql.BrandEpisodes, error) {
	if err := acquire(ctx, requests); err != nil {
		return nil, err
	}
	defer release(requests)

	return p.client.BrandEpisodes(ctx, brandID, limit)
}

func (p *Provider) audio(
	ctx context.Context,
	publicID int,
	requests chan struct{},
) (*api.Audio, error) {
	if err := acquire(ctx, requests); err != nil {
		return nil, err
	}
	defer release(requests)

	return p.client.Audio(ctx, publicID)
}

// extractNumber extracts the last numeric segment from a string
//
//	"62250" -> 62250
//	"https://smotrim.ru/brand/57166" -> 57166
func (p *Provider) extractNumber(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")

	if s == "" {
		return 0, errors.New("empty string")
	}

	// Find the last path segment.
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse number from %q: %w", s, err)
	}

	return n, nil
}

// acquire blocks until a slot on sem is available, or returns ctx's error
// if ctx is done first.
func acquire(ctx context.Context, sem chan struct{}) error {
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release frees a slot acquired via acquire.
func release(sem chan struct{}) {
	<-sem
}
