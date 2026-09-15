package feed_provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/feed-relay/contracts"
	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/internal/media"
)

//go:generate moq --out ./mocks/client_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Client

const Platform = "smotrim"

const feedWorkers = 4

const clientRequests = 8

// Client is the Smotrim API client used by Provider.
//
// NOTE: reconstructed for testing purposes from its two call sites in the
// original code (p.client.BrandEpisodes / p.client.Audio) - point this at
// the real client interface/type if the name or signatures differ.
type Client interface {
	BrandEpisodes(ctx context.Context, brandID, perShowLimit int) (*graphql.BrandEpisodes, error)
	Audio(ctx context.Context, publicID int) (*api.Audio, error)
}

// Feed is the subset of contracts.Feed that Feeds/feed need.
type Feed interface {
	Limit() int
	Shows() []string
	Link() string
	Slug() string
	Title() string
	Description() string
	Image() string
}

// feedTask is one feed to be processed by a worker; all of its
// shows are merged into a single feed.
type feedTask struct {
	feed Feed
}

type Provider struct {
	client  Client
	adapter Adapter
}

// NewProvider builds a Provider.
//
// NOTE for review: same as NewAdapter - Provider's fields are unexported,
// so black-box tests need a constructor. Add this if one doesn't already
// exist under a different name.
func NewProvider(config Config) *Provider {
	var client Client
	var sizer FileSizer

	if config.TestData() {
		client = api.NewTestdataClient()
		sizer = &media.EmptySizer{}
	} else {
		client = api.NewClient(config.HTTPTimeout())
		sizer = media.NewHttpFileSizer(4)
	}

	return &Provider{
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

func (p *Provider) extractNumber(show string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(show, "%d", &n); err != nil {
		return 0, fmt.Errorf("smotrim: %q has no number: %w", show, err)
	}
	return n, nil
}

func (p *Provider) brandEpisodes(ctx context.Context, brandID, perShowLimit int, requests chan struct{}) (*graphql.BrandEpisodes, error) {
	if err := acquire(ctx, requests); err != nil {
		return nil, err
	}
	defer release(requests)

	return p.client.BrandEpisodes(ctx, brandID, perShowLimit)
}

func (p *Provider) audio(ctx context.Context, publicID int, requests chan struct{}) (*api.Audio, error) {
	if err := acquire(ctx, requests); err != nil {
		return nil, err
	}
	defer release(requests)

	return p.client.Audio(ctx, publicID)
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

		go func(episode *graphql.Episode) {
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
		}(episode)
	}

	wg.Wait()

	return audioData, errors.Join(errs...)
}

func (p *Provider) feed(ctx context.Context, feed Feed, requests chan struct{}) (string, *rsscast.Feed, error) {
	showIDs := feed.Shows()

	results := make([]graphql.Show, len(showIDs))
	errs := make([]error, len(showIDs))

	var wg sync.WaitGroup
	for i, show := range showIDs {
		wg.Add(1)

		go func(i int, show string) {
			defer wg.Done()

			brandID, err := p.extractNumber(show)
			if err != nil {
				errs[i] = fmt.Errorf("show %q: extract number failed: %w", show, err)
				return
			}

			brandEpisodes, err := p.brandEpisodes(ctx, brandID, feed.Limit(), requests)
			if err != nil {
				errs[i] = fmt.Errorf("show %q: fetch brand episodes failed: %w", show, err)
				return
			}

			if brandEpisodes == nil || brandEpisodes.Brand == nil {
				errs[i] = fmt.Errorf("smotrim: brand %d not found", brandID)
				return
			}

			if len(brandEpisodes.Brand.Channels) == 0 {
				errs[i] = fmt.Errorf("smotrim: brand %d has no channels", brandID)
				return
			}

			results[i] = graphql.Show{
				Channel:  &brandEpisodes.Brand.Channels[0],
				Brand:    brandEpisodes.Brand,
				Episodes: brandEpisodes.Episodes,
			}
		}(i, show)
	}
	wg.Wait()

	fetchErr := errors.Join(errs...)

	var shows []graphql.Show
	for _, r := range results {
		if r.Brand != nil {
			shows = append(shows, r)
		}
	}

	if len(shows) == 0 {
		return "", nil, fetchErr
	}

	var allEpisodes []*graphql.Episode
	for _, r := range shows {
		allEpisodes = append(allEpisodes, r.Episodes...)
	}

	audioData, err := p.audioData(ctx, allEpisodes, requests)
	if err != nil {
		slog.Error("smotrim: audio data err", "err", err)
	}

	rssFeed, err := p.adapter.Feed(ctx, feed, shows, audioData)
	if err != nil {
		return "", rssFeed, errors.Join(fmt.Errorf("build feed failed: %w", err), fetchErr)
	}

	slug := fmt.Sprintf("%s-%s", p.Platform(), feed.Slug())

	return slug, rssFeed, fetchErr
}

func (p *Provider) Feeds(ctx context.Context, feeds []contracts.Feed) (map[string]*rsscast.Feed, error) {
	var allTasks []feedTask
	for _, f := range feeds {
		if len(f.Shows()) == 0 {
			continue
		}
		allTasks = append(allTasks, feedTask{feed: f})
	}

	workers := min(feedWorkers, len(allTasks))
	if workers == 0 {
		return nil, errors.New("smotrim: no shows")
	}

	tasks := make(chan feedTask)

	var wg sync.WaitGroup
	var mu sync.Mutex

	rssFeeds := make(map[string]*rsscast.Feed)
	var errs []error

	// Limits the total number of concurrent requests to p.client,
	// including BrandEpisodes and Audio calls, across every show of
	// every subscription being processed right now.
	requests := make(chan struct{}, clientRequests)

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for task := range tasks {
				slug, feed, err := p.feed(ctx, task.feed, requests)
				if err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
				if feed == nil {
					continue
				}

				mu.Lock()
				rssFeeds[slug] = feed
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

	if len(rssFeeds) > 0 {
		return rssFeeds, err
	}

	return nil, err
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
