package feed_provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/pkg/feed_provider/mocks"
)

// These tests exercise Adapter.Feed only through its exported contract
// (via NewAdapter), using the real moq-generated mocks. The
// trickier merge/sort/skip logic (flatten, airDate, episodeAudio, ...)
// has its own direct, white-box tests in internal_test.go - this file
// checks Feed's validation errors and a happy-path smoke test, since
// package provider_test can't reach *rsscast.Feed's internals to assert
// on built items directly.

func newTestAdapter() Adapter {
	cfg := &mocks.ConfigMock{
		GeneratorFunc:        func() string { return "smotrim-feed-provider" },
		ItunesOwnerNameFunc:  func() string { return "Smotrim Feeds" },
		ItunesOwnerEmailFunc: func() string { return "feeds@example.com" },
	}
	fs := &mocks.FileSizerMock{
		SizesFunc: func(_ context.Context, urls []string) map[string]int64 {
			sizes := make(map[string]int64, len(urls))
			for _, u := range urls {
				sizes[u] = 1000
			}
			return sizes
		},
	}
	return NewAdapter(cfg, fs)
}

func TestAdapterFeed_ValidationErrors(t *testing.T) {
	validChannel := &graphql.Channel{Title: "channel"}
	validBrand := &graphql.Brand{ID: 1, Title: "brand"}
	datedEpisode := &graphql.Episode{Title: "ep", AirDate: graphql.NewSmotrimTime(time.Now())}

	tests := []struct {
		name      string
		shows     []graphql.Show
		wantInErr string
	}{
		{
			name:      "no shows",
			shows:     nil,
			wantInErr: "no shows provided",
		},
		{
			name: "primary show has nil channel",
			shows: []graphql.Show{
				{Channel: nil, Brand: validBrand, Episodes: []*graphql.Episode{datedEpisode}},
			},
			wantInErr: "channel is nil",
		},
		{
			name: "primary show has nil brand",
			shows: []graphql.Show{
				{Channel: validChannel, Brand: nil, Episodes: []*graphql.Episode{datedEpisode}},
			},
			wantInErr: "brand is nil",
		},
		{
			name: "no episodes across any show",
			shows: []graphql.Show{
				{Channel: validChannel, Brand: validBrand, Episodes: nil},
			},
			wantInErr: "no episodes provided",
		},
		{
			name: "no episode has an air date",
			shows: []graphql.Show{
				{Channel: validChannel, Brand: validBrand, Episodes: []*graphql.Episode{{Title: "undated"}}},
			},
			wantInErr: "no non-nil episode to seed pubDate from",
		},
	}

	a := newTestAdapter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed, err := a.Feed(context.Background(), mocks.FakeFeed{}, tt.shows, map[int]*api.Audio{})
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantInErr)
			assert.Nil(t, feed)
		})
	}
}

func TestAdapterFeed_HappyPath(t *testing.T) {
	shows := []graphql.Show{
		{
			Channel: &graphql.Channel{Title: "channel-1"},
			Brand:   &graphql.Brand{ID: 1, Title: "brand-1", Description: "desc-1"},
			Episodes: []*graphql.Episode{
				{
					Title:   "ep-1",
					AirDate: graphql.NewSmotrimTime(time.Now().Add(-1 * time.Hour)),
					Audio:   &graphql.Audio{PublicId: 1},
				},
			},
		},
		{
			Channel: &graphql.Channel{Title: "channel-2"},
			Brand:   &graphql.Brand{ID: 2, Title: "brand-2", Description: "desc-2"},
			Episodes: []*graphql.Episode{
				{
					Title:   "ep-2",
					AirDate: graphql.NewSmotrimTime(time.Now().Add(-2 * time.Hour)),
					Audio:   &graphql.Audio{PublicId: 2},
				},
			},
		},
	}
	audios := map[int]*api.Audio{
		1: {ShareLink: "https://smotrim.ru/share/1", Streams: &api.Streams{Mp3: "https://smotrim.ru/1.mp3"}, Duration: 1800},
		2: {ShareLink: "https://smotrim.ru/share/2", Streams: &api.Streams{Mp3: "https://smotrim.ru/2.mp3"}, Duration: 1900},
	}

	a := newTestAdapter()
	feed, err := a.Feed(context.Background(), mocks.FakeFeed{}, shows, audios)

	require.NoError(t, err)
	require.NotNil(t, feed)
}
