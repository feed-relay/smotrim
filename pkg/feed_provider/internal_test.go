package feed_provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
)

// These tests live in package provider (white-box) specifically to reach
// unexported helpers (flatten, airDate, episodeAudio, firstAirDate,
// length, resolveSizes). Because of that they deliberately do NOT import
// provider/mocks: that package imports provider for the Show/Adapter
// types in its generated signatures, and Go rejects a test file in
// package provider importing anything that imports provider back
// ("import cycle not allowed in test"). Contract-level tests that need
// the moq mocks live in adapter_test.go / provider_test.go instead, as
// package provider_test.

func TestAdapterFlatten(t *testing.T) {
	ch1 := &graphql.Channel{Title: "channel-1"}
	ch2 := &graphql.Channel{Title: "channel-2"}
	brand1 := &graphql.Brand{ID: 1, Title: "brand-1"}
	brand2 := &graphql.Brand{ID: 2, Title: "brand-2"}

	now := time.Now()
	epNewest := &graphql.Episode{Title: "newest", AirDate: graphql.NewAirDate(now.Add(-1 * time.Hour))}
	epMiddle := &graphql.Episode{Title: "middle", AirDate: graphql.NewAirDate(now.Add(-2 * time.Hour))}
	epOldest := &graphql.Episode{Title: "oldest", AirDate: graphql.NewAirDate(now.Add(-3 * time.Hour))}
	epUndated := &graphql.Episode{Title: "undated"}

	// Deliberately NOT pre-sorted and interleaved across shows, so a bug
	// that just concatenates show order instead of sorting by date would
	// fail this test.
	shows := []Show{
		{Channel: ch1, Brand: brand1, Episodes: []*graphql.Episode{epMiddle, epUndated}},
		{Channel: ch2, Brand: brand2, Episodes: []*graphql.Episode{epNewest, epOldest}},
	}

	a := &adapter{}
	got := a.flatten(shows)

	require.Len(t, got, 4)
	assert.Same(t, epNewest, got[0].episode)
	assert.Same(t, ch2, got[0].channel)
	assert.Same(t, epMiddle, got[1].episode)
	assert.Same(t, ch1, got[1].channel)
	assert.Same(t, epOldest, got[2].episode)
	assert.Same(t, ch2, got[2].channel)
	// Undated episodes sort after every dated one, regardless of source show.
	assert.Same(t, epUndated, got[3].episode)
	assert.Same(t, ch1, got[3].channel)
}

func TestAdapterFlattenSkipsShowsMissingChannelOrBrand(t *testing.T) {
	ep := &graphql.Episode{Title: "ep"}
	shows := []Show{
		{Channel: nil, Brand: &graphql.Brand{}, Episodes: []*graphql.Episode{ep}},
		{Channel: &graphql.Channel{}, Brand: nil, Episodes: []*graphql.Episode{ep}},
	}

	a := &adapter{}
	got := a.flatten(shows)

	assert.Empty(t, got)
}

func TestAdapterAirDate(t *testing.T) {
	a := &adapter{}

	t.Run("nil episode", func(t *testing.T) {
		_, ok := a.airDate(nil)
		assert.False(t, ok)
	})

	t.Run("nil AirDate", func(t *testing.T) {
		_, ok := a.airDate(&graphql.Episode{})
		assert.False(t, ok)
	})

	t.Run("set AirDate", func(t *testing.T) {
		want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		got, ok := a.airDate(&graphql.Episode{AirDate: graphql.NewAirDate(want)})
		require.True(t, ok)
		assert.True(t, want.Equal(got))
	})
}

func TestAdapterEpisodeAudio(t *testing.T) {
	a := &adapter{}
	audio1 := &api.Audio{Streams: api.AudioStreams{Mp3: "https://example.com/1.mp3"}}
	audioEmptyStream := &api.Audio{Streams: api.AudioStreams{Mp3: ""}}
	audios := map[int]*api.Audio{
		1: audio1,
		2: audioEmptyStream,
	}

	tests := []struct {
		name    string
		episode *graphql.Episode
		want    *api.Audio
	}{
		{"nil episode", nil, nil},
		{"nil audio ref", &graphql.Episode{}, nil},
		{"not in audios map", &graphql.Episode{Audio: &graphql.AudioRef{PublicId: 999}}, nil},
		{"empty mp3 stream", &graphql.Episode{Audio: &graphql.AudioRef{PublicId: 2}}, nil},
		{"found", &graphql.Episode{Audio: &graphql.AudioRef{PublicId: 1}}, audio1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.episodeAudio(tt.episode, audios)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Same(t, tt.want, got)
		})
	}
}

func TestAdapterFirstAirDate(t *testing.T) {
	a := &adapter{}

	t.Run("empty slice", func(t *testing.T) {
		_, ok := a.firstAirDate(nil)
		assert.False(t, ok)
	})

	t.Run("nil and undated only", func(t *testing.T) {
		eps := []*graphql.Episode{nil, {Title: "no-date"}}
		_, ok := a.firstAirDate(eps)
		assert.False(t, ok)
	})

	t.Run("skips leading nil/undated entries", func(t *testing.T) {
		want := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		eps := []*graphql.Episode{
			nil,
			{Title: "no-date"},
			{Title: "dated", AirDate: graphql.NewAirDate(want)},
		}
		got, ok := a.firstAirDate(eps)
		require.True(t, ok)
		assert.True(t, want.Equal(got))
	})
}

func TestAdapterLength(t *testing.T) {
	a := &adapter{}
	audio := &api.Audio{Streams: api.AudioStreams{Mp3: "https://example.com/a.mp3"}}

	t.Run("known positive size", func(t *testing.T) {
		sizes := map[string]int64{"https://example.com/a.mp3": 12345}
		assert.EqualValues(t, 12345, a.length(sizes, audio))
	})

	t.Run("missing from sizes", func(t *testing.T) {
		assert.EqualValues(t, defaultLength, a.length(map[string]int64{}, audio))
	})

	t.Run("zero or negative falls back to default", func(t *testing.T) {
		sizes := map[string]int64{"https://example.com/a.mp3": 0}
		assert.EqualValues(t, defaultLength, a.length(sizes, audio))
	})
}

// fakeFileSizer is a hand-written test double, not a moq mock: it's used
// only from this white-box file, which can't import provider/mocks (see
// the package comment above).
type fakeFileSizer struct {
	gotURLs []string
	sizes   map[string]int64
}

func (f *fakeFileSizer) Sizes(_ context.Context, urls []string) map[string]int64 {
	f.gotURLs = urls
	return f.sizes
}

func TestAdapterResolveSizes(t *testing.T) {
	audios := map[int]*api.Audio{
		1: {Streams: api.AudioStreams{Mp3: "https://example.com/1.mp3"}},
		2: {Streams: api.AudioStreams{Mp3: "https://example.com/2.mp3"}},
	}
	episodes := []*graphql.Episode{
		{Audio: &graphql.AudioRef{PublicId: 1}},
		{Audio: &graphql.AudioRef{PublicId: 2}},
		{Audio: &graphql.AudioRef{PublicId: 1}},   // duplicate URL - must be deduped
		{Audio: nil},                              // no audio - skipped
		{Audio: &graphql.AudioRef{PublicId: 999}}, // not in audios - skipped
	}

	fs := &fakeFileSizer{sizes: map[string]int64{"https://example.com/1.mp3": 111}}
	a := &adapter{fileSizer: fs}

	got := a.resolveSizes(context.Background(), episodes, audios)

	assert.ElementsMatch(t, []string{"https://example.com/1.mp3", "https://example.com/2.mp3"}, fs.gotURLs)
	assert.Equal(t, fs.sizes, got)
}

func TestAcquireRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // fill the only slot so the next acquire has to wait

	cancel()

	err := acquire(ctx, sem)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
