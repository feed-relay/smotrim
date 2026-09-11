package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/feed-relay/rsscast"
)

type FeedProvider struct{}

type feed interface {
	Limit() int
	Shows() []string
	Slug() string
}

type feedsTask struct {
	feed feed
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
	return "", nil, nil
}
