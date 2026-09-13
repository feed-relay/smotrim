package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/pkg/provider/mocks"
)

// ---- fixture helpers -------------------------------------------------
//
// Fixtures are built by unmarshalling JSON that mirrors the real GraphQL
// response shape (same json tags as graphql.Channel/graphql.Brand/graphql.Episode). This
// avoids hard-coding the exact Go types of nested fields (Image,
// ImagePreset, Season, the Episode.Audio reference) in the test file,
// while still exercising the real json tags and SmotrimTime parsing.

func mustChannel(t *testing.T, js string) *graphql.Channel {
	t.Helper()
	var c graphql.Channel
	require.NoError(t, json.Unmarshal([]byte(js), &c))
	return &c
}

func mustBrand(t *testing.T, js string) *graphql.Brand {
	t.Helper()
	var b graphql.Brand
	require.NoError(t, json.Unmarshal([]byte(js), &b))
	return &b
}

func mustEpisode(t *testing.T, js string) *graphql.Episode {
	t.Helper()
	var e graphql.Episode
	require.NoError(t, json.Unmarshal([]byte(js), &e))
	return &e
}

// newAudio builds a smotrim Audio (the full player-API object, keyed into
// map[int]*api.Audio by public ID) - not to be confused with the lightweight
// graphql.Audio reference embedded in Episode.
func newAudio(publicId int, shareLink, mp3 string, duration int64) *api.Audio {
	a := &api.Audio{PublicId: publicId, ShareLink: shareLink, Duration: duration, Streams: &api.Streams{}}
	a.Streams.Mp3 = mp3
	return a
}

func newConfigMock() *mocks.ConfigMock {
	return &mocks.ConfigMock{
		GeneratorFunc:        func() string { return "smotrim-generator" },
		ItunesOwnerNameFunc:  func() string { return "Owner Name" },
		ItunesOwnerEmailFunc: func() string { return "owner@example.com" },
	}
}

func newFileSizerMock(sizes map[string]int64) *mocks.FileSizerMock {
	return &mocks.FileSizerMock{
		SizesFunc: func(ctx context.Context, urls []string) map[string]int64 {
			return sizes
		},
	}
}

const testChannelJSON = `{"id":1,"title":"Радио Россия","slug":"radio-rossii"}`
const testBrandJSON = `{
	"id": 67390,
	"title": "Имя и время",
	"description": "О людях науки",
	"images": [{"presets": [{"name": "large", "link": "https://cdn.example/brand.png"}]}]
}`
const testBrandNoImagesJSON = `{"id": 67390, "title": "Имя и время", "description": "О людях науки"}`

// episode with audio, season, number and an image - the full happy path.
const testEpisodeFullJSON = `{
	"id": 3590319,
	"title": "Сальери от мира науки",
	"description": "Выпуск про...",
	"airDate": "2025-12-22T14:35:00+03:00",
	"number": 25,
	"season": {"number": 2025},
	"audio": {"publicId": 2883598, "duration": 586},
	"images": [{"presets": [{"name": "large", "link": "https://cdn.example/ep.png"}]}]
}`

// ---- validation guards -------------------------------------------------

func TestFeed_NilChannel_ReturnsError(t *testing.T) {
	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(nil)}

	feed, err := a.Feed(context.Background(), nil, mustBrand(t, testBrandJSON),
		[]*graphql.Episode{mustEpisode(t, testEpisodeFullJSON)}, map[int]*api.Audio{})

	require.Error(t, err)
	assert.Nil(t, feed)
}

func TestFeed_NilBrand_ReturnsError(t *testing.T) {
	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(nil)}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON), nil,
		[]*graphql.Episode{mustEpisode(t, testEpisodeFullJSON)}, map[int]*api.Audio{})

	require.Error(t, err)
	assert.Nil(t, feed)
}

func TestFeed_NoEpisodes_ReturnsError(t *testing.T) {
	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(nil)}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), nil, map[int]*api.Audio{})

	require.Error(t, err)
	assert.Nil(t, feed)
}

// ---- per-episode resilience (the panics from the original code) -------

func TestFeed_SkipsNilEpisodeEntries(t *testing.T) {
	audio := newAudio(2883598, "https://smotrim.ru/share/1", "https://cdn.example/1.mp3", 586)
	audios := map[int]*api.Audio{2883598: audio}
	valid := mustEpisode(t, testEpisodeFullJSON)

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{nil, valid, nil}, audios)

	require.NoError(t, err)
	require.NotNil(t, feed)
	assert.Equal(t, 1, itemCount(t, feed))
}

func TestFeed_SkipsEpisodeWithoutLinkedAudio(t *testing.T) {
	// episode.Audio is nil (no "audio" key) - must not panic, must be skipped.
	noAudio := mustEpisode(t, `{"id":1,"title":"черновик","airDate":"2025-12-22T14:35:00+03:00"}`)

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{noAudio}, map[int]*api.Audio{})

	require.NoError(t, err, "Feed itself must not fail/panic when an episode has no linked audio")
	require.NotNil(t, feed)
	// No valid episode means no item was added; rsscast's own Validate
	// confirms zero items, rather than us reaching into unexported fields.
	assert.ErrorContains(t, feed.Validate(), "at least one channel item is required")
}

func TestFeed_SkipsEpisodeMissingFromAudioData(t *testing.T) {
	// episode references a publicId that has no entry in audios.
	ep := mustEpisode(t, `{"id":1,"title":"ep","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":404}}`)

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep}, map[int]*api.Audio{} /* empty */)

	require.NoError(t, err, "Feed itself must not fail/panic when audio is missing from AudioData")
	require.NotNil(t, feed)
	assert.ErrorContains(t, feed.Validate(), "at least one channel item is required")
}

func TestFeed_OmitsSeasonWhenNil(t *testing.T) {
	// no "season" key at all -> episode.Season is nil.
	ep := mustEpisode(t, `{"id":1,"title":"ep","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":1}}`)
	audios := map[int]*api.Audio{1: newAudio(1, "https://smotrim.ru/1", "https://cdn.example/1.mp3", 100)}

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep}, audios)

	require.NoError(t, err)
	xmlStr := encode(t, feed)
	assert.NotContains(t, xmlStr, "itunes:season")
}

func TestFeed_OmitsBrandImageWhenMissing(t *testing.T) {
	ep := mustEpisode(t, `{"id":1,"title":"ep","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":1}}`)
	audios := map[int]*api.Audio{1: newAudio(1, "https://smotrim.ru/1", "https://cdn.example/1.mp3", 100)}

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	// brand has no Images at all - must not panic building the feed.
	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandNoImagesJSON), []*graphql.Episode{ep}, audios)

	require.NoError(t, err, "Feed itself must not panic when brand has no images")
	require.NotNil(t, feed)
	// Apple Podcasts requires a channel-level itunes:image; rsscast now
	// surfaces that as a clean, catchable Validate() error instead of
	// adapter.go crashing the whole process with an index-out-of-range panic.
	assert.ErrorContains(t, feed.Validate(), "itunes:image")
}

func TestFeed_OmitsEpisodeImageWhenMissing(t *testing.T) {
	// episode has no "images" key - must not panic, and produces a valid feed
	// since itunes:image is optional at the item level (unlike channel level).
	ep := mustEpisode(t, `{"id":1,"title":"ep","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":1}}`)
	audios := map[int]*api.Audio{1: newAudio(1, "https://smotrim.ru/1", "https://cdn.example/1.mp3", 100)}

	a := &adapter{config: newConfigMock(), fileSizer: newFileSizerMock(map[string]int64{})}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep}, audios)

	require.NoError(t, err)
	require.NoError(t, feed.Validate())
	xmlStr := encode(t, feed)
	// Only the channel-level <itunes:image> (from the brand) should be
	// present; the episode contributes no item-level one.
	assert.Equal(t, 1, strings.Count(xmlStr, "<itunes:image"))
}

// ---- happy path ----------------------------------------------------------

func TestFeed_Success_EncodesValidFeed(t *testing.T) {
	ep1 := mustEpisode(t, testEpisodeFullJSON)
	ep2 := mustEpisode(t, `{
		"id": 2, "title": "Второй выпуск", "airDate": "2025-12-23T14:35:00+03:00",
		"audio": {"publicId": 111}
	}`)
	audios := map[int]*api.Audio{
		2883598: newAudio(2883598, "https://smotrim.ru/share/1", "https://cdn.example/1.mp3", 586),
		111:     newAudio(111, "https://smotrim.ru/share/2", "https://cdn.example/2.mp3", 300),
	}
	cfg := newConfigMock()
	fs := newFileSizerMock(map[string]int64{"https://cdn.example/1.mp3": 12_345_678})

	a := &adapter{config: cfg, fileSizer: fs}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep1, ep2}, audios)

	require.NoError(t, err)
	require.NotNil(t, feed)
	require.NoError(t, feed.Validate())

	xmlStr := encode(t, feed)
	assert.Equal(t, 2, strings.Count(xmlStr, "<item>"))
	assert.Contains(t, xmlStr, "Сальери от мира науки")
	assert.Contains(t, xmlStr, "https://cdn.example/1.mp3")
	assert.Contains(t, xmlStr, "https://cdn.example/2.mp3")
	assert.Contains(t, xmlStr, "12345678") // resolved size, not defaultLength
	assert.Contains(t, xmlStr, "1000000")  // ep2's size falls back to defaultLength

	// config was consulted for the feed-level tags.
	assert.Len(t, cfg.GeneratorCalls(), 1)
	assert.Len(t, cfg.ItunesOwnerNameCalls(), 1)
	assert.Len(t, cfg.ItunesOwnerEmailCalls(), 1)
}

func TestFeed_DedupesFileSizerURLs(t *testing.T) {
	// two episodes share the same audio publicId / mp3 URL.
	ep1 := mustEpisode(t, `{"id":1,"title":"e1","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":1}}`)
	ep2 := mustEpisode(t, `{"id":2,"title":"e2","airDate":"2025-12-22T14:36:00+03:00","audio":{"publicId":1}}`)
	audios := map[int]*api.Audio{1: newAudio(1, "https://smotrim.ru/1", "https://cdn.example/same.mp3", 100)}

	fs := newFileSizerMock(map[string]int64{})
	a := &adapter{config: newConfigMock(), fileSizer: fs}

	_, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep1, ep2}, audios)

	require.NoError(t, err)
	calls := fs.SizesCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"https://cdn.example/same.mp3"}, calls[0].Urls)
}

func TestFeed_FallsBackToDefaultLengthWhenSizeNonPositive(t *testing.T) {
	ep := mustEpisode(t, `{"id":1,"title":"e1","airDate":"2025-12-22T14:35:00+03:00","audio":{"publicId":1}}`)
	audios := map[int]*api.Audio{1: newAudio(1, "https://smotrim.ru/1", "https://cdn.example/1.mp3", 100)}

	// FileSizer returns a non-positive size - should still fall back.
	fs := newFileSizerMock(map[string]int64{"https://cdn.example/1.mp3": 0})
	a := &adapter{config: newConfigMock(), fileSizer: fs}

	feed, err := a.Feed(context.Background(), mustChannel(t, testChannelJSON),
		mustBrand(t, testBrandJSON), []*graphql.Episode{ep}, audios)

	require.NoError(t, err)
	xmlStr := encode(t, feed)
	assert.Contains(t, xmlStr, `length="1000000"`)
}

// ---- test helpers ---------------------------------------------------------

func encode(t *testing.T, feed *rsscast.Feed) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, feed.Encode(&buf))
	return buf.String()
}

func itemCount(t *testing.T, feed *rsscast.Feed) int {
	t.Helper()
	return strings.Count(encode(t, feed), "<item>")
}
