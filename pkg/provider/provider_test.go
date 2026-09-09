package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/feed-relay/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/rsscast"

	"github.com/feed-relay/smotrim/internal/api"
	"github.com/feed-relay/smotrim/internal/api/graphql"
	"github.com/feed-relay/smotrim/pkg/provider/mocks"
)

func TestProvider_Feeds_NoShows(t *testing.T) {
	p := &Provider{}

	subscription := &mocks.SubMock{
		ShowsFunc: func() []string {
			return nil
		},
	}

	feeds, err := p.Feeds(context.Background(), []contracts.Subscription{
		subscription,
	})

	require.Nil(t, feeds)
	require.EqualError(t, err, "smotrim: no shows")
}

func TestProvider_Feeds_Success(t *testing.T) {
	ctx := context.Background()

	brandID := 123
	audioID := 456
	limit := 10

	brand := &graphql.Brand{
		ID:    brandID,
		Title: "Test brand",
		Channels: []graphql.Channel{
			{
				ID:    1,
				Title: "Test channel",
				Slug:  "test-channel",
			},
		},
	}

	episode := &graphql.Episode{
		ID:    100,
		Title: "Episode 1",
		Audio: &graphql.Audio{
			PublicId: audioID,
		},
	}

	audio := &api.Audio{
		PublicId: audioID,
	}

	expectedFeed := rsscast.NewFeed(rsscast.FeedData{
		Title:       "Test feed",
		Description: "Test feed",
		Language:    "en",
		Explicit:    rsscast.ExplicitFalse,
		Categories: []rsscast.Category{
			rsscast.NewCategory("Technology"),
		},
	})

	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			gotLimit int,
		) (*graphql.BrandEpisodes, error) {
			assert.Equal(t, brandID, id)
			assert.Equal(t, limit, gotLimit)

			return &graphql.BrandEpisodes{
				Brand:    brand,
				Episodes: []*graphql.Episode{episode},
			}, nil
		},
		AudioFunc: func(ctx context.Context, publicID int) (*api.Audio, error) {
			assert.Equal(t, audioID, publicID)

			return audio, nil
		},
	}

	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(
			ctx context.Context,
			channel *graphql.Channel,
			gotBrand *graphql.Brand,
			episodes []*graphql.Episode,
			audios map[int]*api.Audio,
		) (*rsscast.Feed, error) {
			assert.Equal(t, &brand.Channels[0], channel)
			assert.Same(t, brand, gotBrand)
			assert.Equal(t, []*graphql.Episode{episode}, episodes)
			assert.Equal(t, map[int]*api.Audio{
				audioID: audio,
			}, audios)

			return expectedFeed, nil
		},
	}

	subscriptionMock := &mocks.SubMock{
		LimitFunc: func() int {
			return limit
		},
		ShowsFunc: func() []string {
			return []string{"123"}
		},
	}

	p := &Provider{
		client:  clientMock,
		adapter: adapterMock,
	}

	feeds, err := p.Feeds(ctx, []contracts.Subscription{
		subscriptionMock,
	})

	require.NoError(t, err)
	require.Len(t, feeds, 1)

	actualFeed, ok := feeds["smotrim-brand-123"]
	require.True(t, ok)
	assert.Same(t, expectedFeed, actualFeed)

	require.Len(t, clientMock.BrandEpisodesCalls(), 1)
	require.Len(t, clientMock.AudioCalls(), 1)
	require.Len(t, adapterMock.FeedCalls(), 1)
}

func TestProvider_Feeds_PartialFailure(t *testing.T) {
	goodID := 100
	badID := 200

	expectedErr := errors.New("brand request failed")

	goodBrand := &graphql.Brand{
		ID: goodID,
		Channels: []graphql.Channel{
			{
				ID: 1,
			},
		},
	}

	expectedFeed := rsscast.NewFeed(rsscast.FeedData{
		Title:       "Test feed",
		Description: "Test feed",
		Language:    "en",
		Explicit:    rsscast.ExplicitFalse,
		Categories: []rsscast.Category{
			rsscast.NewCategory("Technology"),
		},
	})

	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			switch id {
			case goodID:
				return &graphql.BrandEpisodes{
					Brand: goodBrand,
				}, nil
			case badID:
				return nil, expectedErr
			default:
				t.Fatalf("unexpected brand id: %d", id)
				return nil, errors.New("unreachable")
			}
		},
	}

	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(
			ctx context.Context,
			channel *graphql.Channel,
			brand *graphql.Brand,
			episodes []*graphql.Episode,
			audios map[int]*api.Audio,
		) (*rsscast.Feed, error) {
			return expectedFeed, nil
		},
	}

	subscriptionMock := &mocks.SubMock{
		LimitFunc: func() int {
			return 10
		},
		ShowsFunc: func() []string {
			return []string{
				"100",
				"200",
			}
		},
	}

	p := &Provider{
		client:  clientMock,
		adapter: adapterMock,
	}

	feeds, err := p.Feeds(
		context.Background(),
		[]contracts.Subscription{subscriptionMock},
	)

	require.Error(t, err)
	require.NotNil(t, feeds)

	assert.Len(t, feeds, 1)
	assert.NotNil(t, feeds["smotrim-brand-100"])
	require.ErrorIs(t, err, expectedErr)
}

func TestProvider_Feeds_AllFailed(t *testing.T) {
	firstErr := errors.New("first error")
	secondErr := errors.New("second error")

	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			switch id {
			case 100:
				return nil, firstErr
			case 200:
				return nil, secondErr
			default:
				t.Fatalf("unexpected brand id: %d", id)
				return nil, errors.New("unreachable")
			}
		},
	}

	subscriptionMock := &mocks.SubMock{
		LimitFunc: func() int {
			return 10
		},
		ShowsFunc: func() []string {
			return []string{"100", "200"}
		},
	}

	p := &Provider{
		client: clientMock,
	}

	feeds, err := p.Feeds(
		context.Background(),
		[]contracts.Subscription{subscriptionMock},
	)

	require.Nil(t, feeds)
	require.Error(t, err)

	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
}

func TestProvider_Feed_BrandNotFound(t *testing.T) {
	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand: nil,
			}, nil
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	_, _, err := p.feed(
		context.Background(),
		&mocks.SubMock{
			LimitFunc: func() int {
				return 10
			},
		},
		"123",
		requests,
	)

	require.EqualError(t, err, "smotrim: brand 123 not found")

	// The semaphore slot must be released after the request.
	require.Empty(t, requests)
}

func TestProvider_Feed_BrandWithoutChannels(t *testing.T) {
	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand: &graphql.Brand{
					ID: 123,
				},
			}, nil
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	_, _, err := p.feed(
		context.Background(),
		&mocks.SubMock{
			LimitFunc: func() int {
				return 10
			},
		},
		"123",
		requests,
	)

	require.EqualError(t, err, "smotrim: brand 123 has no channels")
	require.Empty(t, requests)
}

func TestProvider_Feed_AdapterError(t *testing.T) {
	expectedErr := errors.New("adapter failed")

	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand: &graphql.Brand{
					ID: id,
					Channels: []graphql.Channel{
						{ID: 1},
					},
				},
			}, nil
		},
	}

	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(
			ctx context.Context,
			channel *graphql.Channel,
			brand *graphql.Brand,
			episodes []*graphql.Episode,
			audios map[int]*api.Audio,
		) (*rsscast.Feed, error) {
			return nil, expectedErr
		},
	}

	p := &Provider{
		client:  clientMock,
		adapter: adapterMock,
	}

	_, _, err := p.feed(
		context.Background(),
		&mocks.SubMock{
			LimitFunc: func() int {
				return 10
			},
		},
		"123",
		make(chan struct{}, 1),
	)

	require.ErrorIs(t, err, expectedErr)
}

func TestProvider_AudioData_Success(t *testing.T) {
	audio1 := &api.Audio{PublicId: 101}
	audio2 := &api.Audio{PublicId: 102}

	clientMock := &mocks.ClientMock{
		AudioFunc: func(
			ctx context.Context,
			publicID int,
		) (*api.Audio, error) {
			switch publicID {
			case 101:
				return audio1, nil
			case 102:
				return audio2, nil
			default:
				t.Fatalf("unexpected audio id: %d", publicID)
				return nil, errors.New("unreachable")
			}
		},
	}

	episodes := []*graphql.Episode{
		nil,
		{
			ID: 1,
		},
		{
			ID:    2,
			Audio: nil,
		},
		{
			ID: 3,
			Audio: &graphql.Audio{
				PublicId: 101,
			},
		},
		{
			ID: 4,
			Audio: &graphql.Audio{
				PublicId: 102,
			},
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 2)

	result, err := p.audioData(
		context.Background(),
		episodes,
		requests,
	)

	require.NoError(t, err)
	assert.Equal(t, map[int]*api.Audio{
		101: audio1,
		102: audio2,
	}, result)

	assert.Len(t, clientMock.AudioCalls(), 2)
	assert.Empty(t, requests)
}

func TestProvider_AudioData_PartialFailure(t *testing.T) {
	expectedErr := errors.New("audio request failed")

	successfulAudio := &api.Audio{
		PublicId: 101,
	}

	clientMock := &mocks.ClientMock{
		AudioFunc: func(
			ctx context.Context,
			publicID int,
		) (*api.Audio, error) {
			if publicID == 101 {
				return successfulAudio, nil
			}

			return nil, expectedErr
		},
	}

	episodes := []*graphql.Episode{
		{
			ID: 1,
			Audio: &graphql.Audio{
				PublicId: 101,
			},
		},
		{
			ID: 2,
			Audio: &graphql.Audio{
				PublicId: 102,
			},
		},
	}

	p := &Provider{
		client: clientMock,
	}

	result, err := p.audioData(
		context.Background(),
		episodes,
		make(chan struct{}, 2),
	)

	require.Error(t, err)

	// Successful audio data is preserved even when another request fails.
	assert.Equal(t, map[int]*api.Audio{
		101: successfulAudio,
	}, result)

	assert.ErrorIs(t, err, expectedErr)
	assert.Len(t, clientMock.AudioCalls(), 2)
}

func TestProvider_BrandEpisodes_ReleasesSemaphoreOnSuccess(t *testing.T) {
	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand: &graphql.Brand{
					ID: id,
				},
			}, nil
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	_, err := p.brandEpisodes(
		context.Background(),
		123,
		10,
		requests,
	)

	require.NoError(t, err)
	require.Empty(t, requests)
}

func TestProvider_BrandEpisodes_ReleasesSemaphoreOnError(t *testing.T) {
	expectedErr := errors.New("request failed")

	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return nil, expectedErr
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	_, err := p.brandEpisodes(
		context.Background(),
		123,
		10,
		requests,
	)

	require.ErrorIs(t, err, expectedErr)
	require.Empty(t, requests)
}

func TestProvider_Audio_ReleasesSemaphoreOnSuccess(t *testing.T) {
	expectedAudio := &api.Audio{
		PublicId: 123,
	}

	clientMock := &mocks.ClientMock{
		AudioFunc: func(
			ctx context.Context,
			publicID int,
		) (*api.Audio, error) {
			return expectedAudio, nil
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	result, err := p.audio(
		context.Background(),
		123,
		requests,
	)

	require.NoError(t, err)
	assert.Same(t, expectedAudio, result)
	require.Empty(t, requests)
}

func TestProvider_Audio_ReleasesSemaphoreOnError(t *testing.T) {
	expectedErr := errors.New("audio request failed")

	clientMock := &mocks.ClientMock{
		AudioFunc: func(
			ctx context.Context,
			publicID int,
		) (*api.Audio, error) {
			return nil, expectedErr
		},
	}

	p := &Provider{
		client: clientMock,
	}

	requests := make(chan struct{}, 1)

	_, err := p.audio(
		context.Background(),
		123,
		requests,
	)

	require.ErrorIs(t, err, expectedErr)
	require.Empty(t, requests)
}

func TestAcquire_ContextCancelled(t *testing.T) {
	requests := make(chan struct{}, 1)
	requests <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := acquire(ctx, requests)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Len(t, requests, 1)
}

func TestRelease_FreesSemaphoreSlot(t *testing.T) {
	requests := make(chan struct{}, 1)
	requests <- struct{}{}

	release(requests)

	assert.Empty(t, requests)
}

func TestProvider_Feed_DoesNotLeakRequestSlot(t *testing.T) {
	clientMock := &mocks.ClientMock{
		BrandEpisodesFunc: func(
			ctx context.Context,
			id int,
			limit int,
		) (*graphql.BrandEpisodes, error) {
			return &graphql.BrandEpisodes{
				Brand: &graphql.Brand{
					ID: id,
					Channels: []graphql.Channel{
						{ID: 1},
					},
				},
			}, nil
		},
	}

	adapterMock := &mocks.AdapterMock{
		FeedFunc: func(
			ctx context.Context,
			channel *graphql.Channel,
			brand *graphql.Brand,
			episodes []*graphql.Episode,
			audios map[int]*api.Audio,
		) (*rsscast.Feed, error) {
			return rsscast.NewFeed(rsscast.FeedData{
				Title:       "Test",
				Description: "Test",
				Language:    "en",
				Explicit:    rsscast.ExplicitFalse,
				Categories: []rsscast.Category{
					rsscast.NewCategory("Technology"),
				},
			}), nil
		},
	}

	p := &Provider{
		client:  clientMock,
		adapter: adapterMock,
	}

	requests := make(chan struct{}, 1)

	_, _, err := p.feed(
		context.Background(),
		&mocks.SubMock{
			LimitFunc: func() int {
				return 10
			},
		},
		"123",
		requests,
	)

	require.NoError(t, err)

	// One request must acquire and release exactly one semaphore slot.
	require.Empty(t, requests)
}
