package feed_provider

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/pkg/feed_provider/mocks"
)

// fakeSubscription is a hand-written fixture, not a moq mock: Sub /
// contracts.Subscription is three plain getters with no call-tracking
// worth mocking, so a literal struct reads clearer here. NOTE: this
// assumes contracts.Subscription needs exactly these three methods
// (matching provider.Sub) - if the real interface has more, add stub
// implementations for those too.
type fakeSubscription struct {
	perShowLimit int
	shows        []string
	slug         string
}

func (s fakeSubscription) Limit() int        { return s.perShowLimit }
func (s fakeSubscription) PerShowLimit() int { return s.perShowLimit }
func (s fakeSubscription) Shows() []string   { return s.shows }
func (s fakeSubscription) Slug() string      { return s.slug }

// IMPORTANT: mock *Func closures run on Provider's internal goroutines,
// not the test goroutine - only assert.* (non-fatal) is safe to call
// from inside them; require.* calls t.FailNow(), which the testing
// package documents as goroutine-unsafe. These tests instead let the
// closures just return canned data, then assert afterwards against
// mock.BrandEpisodesCalls() / mock.FeedCalls() / mock.AudioCalls() on
// the main test goroutine.

func TestFeeds_SingleSubscriptionSingleShow(t *testing.T) {
	brand := &graphql.Brand{ID: 42, Title: "brand-1", Channels: []graphql.Channel{{Title: "channel-1"}}}
	episode := &graphql.Episode{Title: "ep-1", Audio: &graphql.Audio{PublicId: 7}}

	client := &mocks.ClientMock{
		BrandEpisodesFunc: func(context.Context, int, int) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{Brand: brand, Episodes: []*graphql.Episode{episode}}, nil
		},
		AudioFunc: func(context.Context, int) (*api.Audio, error) {
			return &api.Audio{Streams: &api.Streams{Mp3: "https://x/1.mp3"}}, nil
		},
	}

	wantFeed := &rsscast.Feed{}
	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(context.Context, []graphql.Show, map[int]*api.Audio) (*rsscast.Feed, error) {
			return wantFeed, nil
		},
	}

	p := NewProvider(client, adapterMock)
	sub := fakeSubscription{perShowLimit: 5, shows: []string{"42"}, slug: "my-sub"}

	feeds, err := p.Feeds(context.Background(), []Sub{sub})
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Same(t, wantFeed, feeds["smotrim-subscription-my-sub"])

	beCalls := client.BrandEpisodesCalls()
	require.Len(t, beCalls, 1)
	assert.Equal(t, 42, beCalls[0].BrandID)
	assert.Equal(t, 5, beCalls[0].PerShowLimit)

	feedCalls := adapterMock.FeedCalls()
	require.Len(t, feedCalls, 1)
	require.Len(t, feedCalls[0].Shows, 1)
	assert.Same(t, brand, feedCalls[0].Shows[0].Brand)
}

func TestFeeds_MultipleSubscriptions(t *testing.T) {
	client := &mocks.ClientMock{
		BrandEpisodesFunc: func(_ context.Context, brandID, _ int) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand:    &graphql.Brand{ID: brandID, Channels: []graphql.Channel{{Title: fmt.Sprintf("channel-%d", brandID)}}},
				Episodes: []*graphql.Episode{{Title: fmt.Sprintf("ep-%d", brandID)}},
			}, nil
		},
	}
	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(context.Context, []graphql.Show, map[int]*api.Audio) (*rsscast.Feed, error) {
			return &rsscast.Feed{}, nil
		},
	}

	p := NewProvider(client, adapterMock)
	subs := []Sub{
		fakeSubscription{perShowLimit: 3, shows: []string{"1"}, slug: "sub-a"},
		fakeSubscription{perShowLimit: 3, shows: []string{"2", "3"}, slug: "sub-b"},
		fakeSubscription{perShowLimit: 3, shows: nil, slug: "sub-empty"}, // must be excluded, not an error
	}

	feeds, err := p.Feeds(context.Background(), subs)

	require.NoError(t, err)
	assert.Len(t, feeds, 2)
	assert.Contains(t, feeds, "smotrim-subscription-sub-a")
	assert.Contains(t, feeds, "smotrim-subscription-sub-b")
	assert.NotContains(t, feeds, "smotrim-subscription-sub-empty")
}

func TestFeeds_NoShowsAnywhereReturnsError(t *testing.T) {
	p := NewProvider(&mocks.ClientMock{}, &mocks.AdapterMock{})

	t.Run("nil subscriptions", func(t *testing.T) {
		feeds, err := p.Feeds(context.Background(), nil)
		assert.Nil(t, feeds)
		assert.ErrorContains(t, err, "no shows")
	})

	t.Run("subscriptions with no shows", func(t *testing.T) {
		subs := []Sub{
			fakeSubscription{slug: "a"},
			fakeSubscription{slug: "b"},
		}
		feeds, err := p.Feeds(context.Background(), subs)
		assert.Nil(t, feeds)
		assert.ErrorContains(t, err, "no shows")
	})
}

func TestFeeds_PartialSuccessWithinSubscription(t *testing.T) {
	okBrand := &graphql.Brand{ID: 2, Title: "brand-2", Channels: []graphql.Channel{{Title: "channel-2"}}}
	okEpisode := &graphql.Episode{Title: "ep-2"}

	client := &mocks.ClientMock{
		BrandEpisodesFunc: func(_ context.Context, brandID, _ int) (*graphql.BrandEpisodes, error) {
			if brandID == 1 {
				return nil, errors.New("upstream unavailable")
			}
			return &graphql.BrandEpisodes{Brand: okBrand, Episodes: []*graphql.Episode{okEpisode}}, nil
		},
	}

	wantFeed := &rsscast.Feed{}
	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(context.Context, []graphql.Show, map[int]*api.Audio) (*rsscast.Feed, error) {
			return wantFeed, nil
		},
	}

	p := NewProvider(client, adapterMock)
	sub := fakeSubscription{perShowLimit: 5, shows: []string{"1", "2"}, slug: "mixed"}

	feeds, err := p.Feeds(context.Background(), []Sub{sub})

	// One show failed, so the error must still surface even though the
	// subscription's feed still gets built from the surviving show.
	require.Error(t, err)
	assert.ErrorContains(t, err, "upstream unavailable")
	require.Len(t, feeds, 1)
	assert.Same(t, wantFeed, feeds["smotrim-subscription-mixed"])

	feedCalls := adapterMock.FeedCalls()
	require.Len(t, feedCalls, 1)
	require.Len(t, feedCalls[0].Shows, 1) // only the surviving show reaches the adapter
	assert.Same(t, okBrand, feedCalls[0].Shows[0].Brand)
}

func TestFeeds_ShowFetchFailureModes(t *testing.T) {
	tests := []struct {
		name              string
		brandEpisodesFunc func(ctx context.Context, brandID, perShowLimit int) (*graphql.BrandEpisodes, error)
	}{
		{
			name: "client error",
			brandEpisodesFunc: func(context.Context, int, int) (*graphql.BrandEpisodes, error) {
				return nil, errors.New("upstream unavailable")
			},
		},
		{
			name: "brand not found",
			brandEpisodesFunc: func(context.Context, int, int) (*graphql.BrandEpisodes, error) {
				return &graphql.BrandEpisodes{Brand: nil}, nil
			},
		},
		{
			name: "brand has no channels",
			brandEpisodesFunc: func(context.Context, int, int) (*graphql.BrandEpisodes, error) {
				return &graphql.BrandEpisodes{Brand: &graphql.Brand{ID: 1, Channels: nil}}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mocks.ClientMock{BrandEpisodesFunc: tt.brandEpisodesFunc}
			// FeedFunc left nil on purpose: Adapter.Feed must not be
			// called when every show in the subscription failed. If it
			// were called, the generated mock panics.
			adapterMock := &mocks.AdapterMock{}

			p := NewProvider(client, adapterMock)
			sub := fakeSubscription{perShowLimit: 5, shows: []string{"1"}, slug: "sub"}

			feeds, err := p.Feeds(context.Background(), []Sub{sub})

			assert.Empty(t, feeds)
			assert.Error(t, err)
			assert.Empty(t, adapterMock.FeedCalls())
		})
	}
}

func TestFeeds_ExtractNumberFailure(t *testing.T) {
	// BrandEpisodesFunc left nil: a show whose ID can't be parsed must
	// never reach the client at all.
	client := &mocks.ClientMock{}
	adapterMock := &mocks.AdapterMock{}
	p := NewProvider(client, adapterMock)

	sub := fakeSubscription{perShowLimit: 5, shows: []string{"not-a-number"}, slug: "sub"}
	feeds, err := p.Feeds(context.Background(), []Sub{sub})

	assert.Nil(t, feeds)
	require.Error(t, err)
	assert.Empty(t, client.BrandEpisodesCalls())
}

func TestFeeds_AdapterFeedError(t *testing.T) {
	client := &mocks.ClientMock{
		BrandEpisodesFunc: func(_ context.Context, brandID, _ int) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand:    &graphql.Brand{ID: brandID, Channels: []graphql.Channel{{Title: "c"}}},
				Episodes: []*graphql.Episode{{Title: "ep"}},
			}, nil
		},
	}
	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(context.Context, []graphql.Show, map[int]*api.Audio) (*rsscast.Feed, error) {
			return nil, errors.New("rss build failed")
		},
	}

	p := NewProvider(client, adapterMock)
	sub := fakeSubscription{perShowLimit: 5, shows: []string{"1"}, slug: "sub"}

	feeds, err := p.Feeds(context.Background(), []Sub{sub})

	assert.Empty(t, feeds)
	require.Error(t, err)
	assert.ErrorContains(t, err, "rss build failed")
}

// TestFeeds_ConcurrencyManySubscriptionsAndShows is meant to be run with
// -race: many subscriptions with varying show counts, some deliberately
// failing, exercising both the outer worker pool and the per-subscription
// per-show goroutines together.
func TestFeeds_ConcurrencyManySubscriptionsAndShows(t *testing.T) {
	client := &mocks.ClientMock{
		BrandEpisodesFunc: func(_ context.Context, brandID, _ int) (*graphql.BrandEpisodes, error) {
			if brandID%7 == 0 {
				return nil, errors.New("simulated upstream error")
			}
			return &graphql.BrandEpisodes{
				Brand: &graphql.Brand{ID: brandID, Channels: []graphql.Channel{{Title: fmt.Sprintf("channel-%d", brandID)}}},
				Episodes: []*graphql.Episode{
					{Title: fmt.Sprintf("ep-%d", brandID), Audio: &graphql.Audio{PublicId: brandID}},
				},
			}, nil
		},
		AudioFunc: func(_ context.Context, publicID int) (*api.Audio, error) {
			return &api.Audio{Streams: &api.Streams{Mp3: fmt.Sprintf("https://x/%d.mp3", publicID)}}, nil
		},
	}
	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(context.Context, []graphql.Show, map[int]*api.Audio) (*rsscast.Feed, error) {
			return &rsscast.Feed{}, nil
		},
	}

	p := NewProvider(client, adapterMock)

	var subs []Sub
	for i := 1; i <= 20; i++ {
		var shows []string
		for j := 0; j < i%5+1; j++ {
			shows = append(shows, fmt.Sprintf("%d", i*10+j))
		}
		subs = append(subs, fakeSubscription{perShowLimit: 3, shows: shows, slug: fmt.Sprintf("sub-%d", i)})
	}

	feeds, err := p.Feeds(context.Background(), subs)

	// Some brandIDs are deliberately %7==0 and fail, so an error is
	// expected here - the point of this test is "doesn't race or
	// deadlock under load", which -race and go test's own timeout cover.
	assert.NotEmpty(t, feeds)
	_ = err
}
