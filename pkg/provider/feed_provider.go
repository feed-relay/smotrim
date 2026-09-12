package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/internal/media"
)

type FeedProvider struct {
	config  Config
	client  Client
	adapter FeedAdapter
}

// Show groups one brand's channel, metadata, and episodes for feed
// generation. A Feed call can combine several Shows into one feed.
type Show struct {
	Channel  *graphql.Channel
	Brand    *graphql.Brand
	Episodes []*graphql.Episode
}

type FeedAdapter interface {
	Feed(ctx context.Context, shows []Show, audios map[int]*api.Audio) (*rsscast.Feed, error)
}

type feed interface {
	LimitPerShow() int
	Shows() []string
	Slug() string
}

type feedsTask struct {
	feed feed
}

// showResult holds what one show contributed to the subscription's feed.
// Left zero-valued (brand == nil) if that show failed — see errs[i].
type showResult struct {
	channel  *graphql.Channel
	brand    *graphql.Brand
	episodes []*graphql.Episode
}

// NewFeedProvider builds a Provider backed by config.
func NewFeedProvider(config Config) *FeedProvider {
	var client Client
	var sizer FileSizer

	if config.TestData() {
		client = api.NewTestdataClient()
		sizer = &media.EmptySizer{}
	} else {
		client = api.NewClient(config.HTTPTimeout())
		sizer = media.NewHttpFileSizer(4)
	}

	return &FeedProvider{
		config: config,
		client: client,
		adapter: &feedAdapter{
			config:    config,
			fileSizer: sizer,
		},
	}
}

func (p *FeedProvider) Platform() string {
	return Platform
}

func (p *FeedProvider) Feeds(ctx context.Context, feeds []feed) (map[string]*rsscast.Feed, error) {
	var allTasks []feedsTask
	for _, f := range feeds {
		if len(f.Shows()) == 0 {
			continue
		}
		allTasks = append(allTasks, feedsTask{feed: f})
	}

	workers := min(feedWorkers, len(allTasks))
	if workers == 0 {
		return nil, errors.New("smotrim: no shows")
	}

	tasks := make(chan feedsTask)

	var wg sync.WaitGroup
	var mu sync.Mutex

	rssFeeds := make(map[string]*rsscast.Feed)
	var errs []error

	// Limits the total number of concurrent requests to p.client,
	// including both BrandEpisodes and Audio calls.
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

func (p *FeedProvider) feed(ctx context.Context, feed feed, requests chan struct{}) (string, *rsscast.Feed, error) {
	showIDs := feed.Shows()

	results := make([]Show, len(showIDs))
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

			brandEpisodes, err := p.brandEpisodes(ctx, brandID, feed.LimitPerShow(), requests)
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

			results[i] = Show{
				Channel:  &brandEpisodes.Brand.Channels[0],
				Brand:    brandEpisodes.Brand,
				Episodes: brandEpisodes.Episodes,
			}
		}(i, show)
	}
	wg.Wait()

	fetchErr := errors.Join(errs...)

	var ok []Show
	for _, r := range results {
		if r.Brand != nil {
			ok = append(ok, r)
		}
	}

	if len(ok) == 0 {
		return "", nil, fetchErr
	}

	var allEpisodes []*graphql.Episode
	for _, r := range ok {
		allEpisodes = append(allEpisodes, r.Episodes...)
	}

	audioData, err := p.audioData(ctx, allEpisodes, requests)
	if err != nil {
		slog.Error("smotrim: audio data err", "err", err)
	}

	f, err := p.adapter.Feed(ctx, ok, audioData)
	if err != nil {
		return "", f, errors.Join(fmt.Errorf("build feed failed: %w", err), fetchErr)
	}

	slug := fmt.Sprintf("%s-%s", p.Platform(), feed.Slug())

	return slug, f, fetchErr
}

// audioData fetches Audio for every episode that has one linked
// (episode.Audio != nil), keyed by audio public ID, concurrently.
//
// Successful results are returned even when one or more requests fail.
// All request errors are joined and returned as a single error.
func (p *FeedProvider) audioData(ctx context.Context, episodes []*graphql.Episode, requests chan struct{}) (map[int]*api.Audio, error) {
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

func (p *FeedProvider) brandEpisodes(ctx context.Context, brandID int, limit int, requests chan struct{}) (*graphql.BrandEpisodes, error) {
	if err := acquire(ctx, requests); err != nil {
		return nil, err
	}
	defer release(requests)

	return p.client.BrandEpisodes(ctx, brandID, limit)
}

func (p *FeedProvider) audio(ctx context.Context, publicID int, requests chan struct{}) (*api.Audio, error) {
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
func (p *FeedProvider) extractNumber(s string) (int, error) {
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
