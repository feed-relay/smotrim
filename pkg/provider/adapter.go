package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
)

//go:generate moq --out ./mocks/filesizer_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . FileSizer

const defaultLength = 1_000_000 // 1 MB

// FileSizer resolves remote media file sizes in bytes.
type FileSizer interface {
	// Sizes returns sizes for the given URLs. Missing or unknown sizes are omitted.
	Sizes(ctx context.Context, urls []string) map[string]int64
}

type adapter struct {
	config    Config
	fileSizer FileSizer
}

// Feed builds an Apple Podcasts-compatible RSS feed from the given data.
// Episodes without a linked audio or matching audio entry are skipped.
func (a *adapter) Feed(
	ctx context.Context,
	channel *graphql.Channel,
	brand *graphql.Brand,
	episodes []*graphql.Episode,
	audios map[int]*api.Audio,
) (*rsscast.Feed, error) {
	if channel == nil {
		return nil, errors.New("smotrim: channel is nil")
	}
	if brand == nil {
		return nil, errors.New("smotrim: brand is nil")
	}
	if len(episodes) == 0 {
		return nil, errors.New("smotrim: no episodes provided")
	}

	pubDate, ok := a.firstAirDate(episodes)
	if !ok {
		return nil, errors.New("smotrim: no non-nil episode to seed pubDate from")
	}

	var feedImage string
	if len(brand.Images) > 0 && len(brand.Images[0].Presets) > 0 {
		feedImage = brand.Images[0].Presets[0].Link
	}

	feedData := rsscast.FeedData{
		Title:       brand.Title,
		Description: brand.Description,
		Image:       feedImage,
		Language:    "ru",
		Explicit:    rsscast.ExplicitFalse,
		Categories: []rsscast.Category{
			rsscast.NewCategory("Society & Culture"),
		},
	}

	feed := rsscast.NewFeed(feedData).
		WithAuthor(channel.Title).
		WithLink(fmt.Sprintf("https://smotrim.ru/brand/%d", brand.ID)).
		WithPubDate(pubDate).
		WithLastBuildDate(time.Now()).
		WithItunesTitle(feedData.Title).
		WithGenerator(a.config.Generator()).
		WithItunesSummary(feedData.Description).
		WithItunesOwner(a.config.ItunesOwnerName(), a.config.ItunesOwnerEmail()).
		WithItunesType(rsscast.TypeEpisodic)

	sizes := a.resolveSizes(ctx, episodes, audios)

	for _, episode := range episodes {
		audio := a.episodeAudio(episode, audios)
		if audio == nil {
			continue
		}

		itemData := rsscast.ItemData{
			Title: episode.Title,
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
			WithItunesAuthor(channel.Title)

		if len(episode.Images) > 0 && len(episode.Images[0].Presets) > 0 {
			item.WithItunesImage(episode.Images[0].Presets[0].Link)
		}

		if episode.Number != 0 {
			item.WithItunesEpisode(episode.Number)
		}

		if episode.Season != nil && episode.Season.Number != 0 {
			item.WithItunesSeason(episode.Season.Number)
		}

		feed.AddItem(item)
	}

	return feed, nil
}

func (a *adapter) episodeAudio(episode *graphql.Episode, audios map[int]*api.Audio) *api.Audio {
	if episode == nil || episode.Audio == nil {
		return nil
	}
	audio := audios[episode.Audio.PublicId]
	if audio == nil || audio.Streams.Mp3 == "" {
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
