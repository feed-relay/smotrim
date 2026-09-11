package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/feed-relay/contracts"
	"github.com/feed-relay/rsscast"
)

type FeedProvider struct{}

func (p *FeedProvider) Feeds(ctx context.Context, subscriptions []contracts.Subscription) (map[string]*rsscast.Feed, error) {
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

func (p *FeedProvider) feed(ctx context.Context, subscription Sub, show string, requests chan struct{}) (string, *rsscast.Feed, error) {
	return "", nil, nil
}
