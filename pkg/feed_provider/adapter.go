package feed_provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/feed-relay/contracts"
	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
)

//go:generate moq --out ./mocks/filesizer_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . FileSizer
//go:generate moq --out ./mocks/config_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Config
//go:generate moq --out ./mocks/adapter_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . Adapter

const defaultLength = 1_000_000 // 1 MB

// FileSizer resolves remote media file sizes in bytes.
type FileSizer interface {
	// Sizes returns sizes for the given URLs. Missing or unknown sizes are omitted.
	Sizes(ctx context.Context, urls []string) map[string]int64
}

// Config supplies feed-level metadata that isn't derived from Smotrim data.
//
// NOTE: reconstructed for testing purposes from its call sites in the
// uploaded adapter.go (Generator/ItunesOwnerName/ItunesOwnerEmail) - point
// this at wherever Config actually lives in the real module if the name
// or method set differs.
type Config interface {
	Generator() string
	ItunesOwnerName() string
	ItunesOwnerEmail() string

	TestData() bool
	HTTPTimeout() time.Duration
}

type Adapter interface {
	Feed(ctx context.Context, feed contracts.Feed, shows []graphql.Show, audios map[int]*api.Audio) (*rsscast.Feed, error)
}

type adapter struct {
	config    Config
	fileSizer FileSizer
}

// NewAdapter builds the default Adapter.
//
// NOTE for review: adapter's fields are unexported, so black-box tests
// (package provider_test) need some constructor to reach them with fake
// Config/FileSizer. Add this if one doesn't already exist under a
// different name - swap the name below to match if it does.
func NewAdapter(config Config, fileSizer FileSizer) Adapter {
	return &adapter{config: config, fileSizer: fileSizer}
}

// mergedEpisode pairs an episode with the channel of the show it came
// from, so itunes:author stays correct once a feed spans several shows.
type mergedEpisode struct {
	brand   *graphql.Brand
	episode *graphql.Episode
	channel *graphql.Channel
}

// Feed builds an Apple Podcasts-compatible RSS feed from the given shows.
// Feed-level metadata (title, description, image, link, author, pubDate)
// comes from the first show, since RSS has no way to express more than
// one; per-episode itunes:author still reflects each episode's own show.
// Episodes without a linked audio or matching audio entry are skipped.
func (a *adapter) Feed(
	ctx context.Context,
	feed contracts.Feed,
	shows []graphql.Show,
	audios map[int]*api.Audio,
) (*rsscast.Feed, error) {
	if len(shows) == 0 {
		return nil, errors.New("smotrim: no shows provided")
	}

	primary := shows[0]
	if primary.Channel == nil {
		return nil, errors.New("smotrim: primary show channel is nil")
	}
	if primary.Brand == nil {
		return nil, errors.New("smotrim: primary show brand is nil")
	}

	merged := a.flatten(shows)
	if len(merged) == 0 {
		return nil, errors.New("smotrim: no episodes provided")
	}

	episodes := make([]*graphql.Episode, len(merged))
	for i, m := range merged {
		episodes[i] = m.episode
	}

	pubDate, ok := a.firstAirDate(episodes)
	if !ok {
		return nil, errors.New("smotrim: no non-nil episode to seed pubDate from")
	}

	feedImage := feed.Image()
	if feedImage == "" {
		if len(primary.Channel.Images) > 0 && len(primary.Channel.Images[0].Presets) > 0 {
			feedImage = primary.Channel.Images[0].Presets[0].Link
		}
	}
	feedTitle := feed.Title()
	if feedTitle == "" {
		feedTitle = primary.Channel.Title
	}
	feedDescription := feed.Description()
	if feedDescription == "" {
		feedDescription = primary.Channel.Description
	}
	if feedDescription == "" {
		feedDescription = feedTitle
	}
	feedLink := feed.Link()
	if feedLink == "" {
		feedLink = fmt.Sprintf("https://smotrim.ru%s", primary.Channel.Slug)
	}

	feedData := rsscast.FeedData{
		Title:       feedTitle,
		Description: feedDescription,
		Image:       feedImage,
		Language:    "ru",
		Explicit:    rsscast.ExplicitFalse,
		// T-ODO genres, subgenres -> rsscast.Category
		Categories: []rsscast.Category{
			rsscast.NewCategory("Society & Culture"),
		},
	}

	rssFeed := rsscast.NewFeed(feedData).
		WithAuthor(primary.Channel.Title).
		WithLink(feedLink).
		WithPubDate(pubDate).
		WithLastBuildDate(time.Now()).
		WithItunesTitle(feedData.Title).
		WithGenerator(a.config.Generator()).
		WithItunesSummary(feedData.Description).
		WithItunesOwner(a.config.ItunesOwnerName(), a.config.ItunesOwnerEmail()).
		WithItunesType(rsscast.TypeEpisodic)

	sizes := a.resolveSizes(ctx, episodes, audios)

	for _, m := range merged {
		episode := m.episode
		audio := a.episodeAudio(episode, audios)
		if audio == nil {
			continue
		}

		var itemTitle string
		if episode.Number > 0 {
			if episode.Season != nil && episode.Season.Number > 0 {
				itemTitle = fmt.Sprintf("%s: [%d-%d] %s", m.brand.Title, episode.Season.Number, episode.Number, episode.Title)
			} else {
				itemTitle = fmt.Sprintf("%s: [%d] %s", m.brand.Title, episode.Number, episode.Title)
			}
		} else {
			itemTitle = fmt.Sprintf("%s: %s", m.brand.Title, episode.Title)
		}
		itemData := rsscast.ItemData{
			Title: itemTitle,
			Guid:  audio.ShareLink,
			Enclosure: rsscast.Enclosure{
				URL:    audio.Streams.Mp3,
				Type:   rsscast.Mp3,
				Length: a.length(sizes, audio),
			},
		}

		item := rsscast.NewItem(itemData)
		if episode.AirDate != nil {
			item.WithPubDate(episode.AirDate.Time())
		}
		item = item.WithDescription(episode.Description).
			WithItunesDuration(audio.Duration).
			WithLink(audio.ShareLink).
			WithItunesExplicit(rsscast.ExplicitFalse).
			WithItunesTitle(itemData.Title).
			WithItunesEpisodeType(rsscast.EpisodeFull).
			WithItunesAuthor(m.brand.Title)

		if len(episode.Images) > 0 && len(episode.Images[0].Presets) > 0 {
			item.WithItunesImage(episode.Images[0].Presets[0].Link)
		}

		if episode.Number != 0 {
			item.WithItunesEpisode(episode.Number)
		}

		if episode.Season != nil && episode.Season.Number != 0 {
			item.WithItunesSeason(episode.Season.Number)
		}

		rssFeed.AddItem(item)
	}

	return rssFeed, nil
}

// flatten merges every show's episodes into one newest-first list, paired
// with the channel of the show each episode came from. firstAirDate
// relies on this order.
func (a *adapter) flatten(shows []graphql.Show) []mergedEpisode {
	var merged []mergedEpisode
	for _, show := range shows {
		if show.Channel == nil || show.Brand == nil {
			continue
		}
		for _, episode := range show.Episodes {
			merged = append(merged, mergedEpisode{brand: show.Brand, episode: episode, channel: show.Channel})
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		di, oki := a.airDate(merged[i].episode)
		dj, okj := a.airDate(merged[j].episode)
		switch {
		case oki && okj:
			return di.After(dj)
		case oki:
			return true
		default:
			return false
		}
	})

	return merged
}

func (a *adapter) airDate(episode *graphql.Episode) (time.Time, bool) {
	if episode == nil || episode.AirDate == nil {
		return time.Time{}, false
	}
	return episode.AirDate.Time(), true
}

func (a *adapter) episodeAudio(episode *graphql.Episode, audios map[int]*api.Audio) *api.Audio {
	if episode == nil || episode.Audio == nil {
		return nil
	}
	audio := audios[episode.Audio.PublicId]
	if audio == nil || audio.Streams == nil || audio.Streams.Mp3 == "" {
		return nil
	}
	return audio
}

// firstAirDate returns the air date of the first non-nil episode.
func (a *adapter) firstAirDate(episodes []*graphql.Episode) (time.Time, bool) {
	for _, episode := range episodes {
		if episode != nil && episode.AirDate != nil {
			return episode.AirDate.Time(), true
		}
	}
	return time.Time{}, false
}

func (a *adapter) resolveSizes(ctx context.Context, episodes []*graphql.Episode, audios map[int]*api.Audio) map[string]int64 {
	seen := make(map[string]struct{}, len(episodes))
	urls := make([]string, 0, len(episodes))

	for _, episode := range episodes {
		audio := a.episodeAudio(episode, audios)
		if audio == nil {
			continue
		}

		url := audio.Streams.Mp3
		if _, ok := seen[url]; ok {
			continue
		}

		seen[url] = struct{}{}
		urls = append(urls, url)
	}

	return a.fileSizer.Sizes(ctx, urls)
}

func (a *adapter) length(sizes map[string]int64, audio *api.Audio) int64 {
	l, ok := sizes[audio.Streams.Mp3]
	if !ok || l <= 0 {
		return defaultLength
	}
	return l
}
