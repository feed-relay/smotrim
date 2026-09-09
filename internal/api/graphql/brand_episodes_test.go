package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_BrandEpisodes_Success(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b

		_, _ = w.Write([]byte(`{
			"data": {
				"brand": {
					"id": 10,
					"title": "Brand Title",
					"description": "Brand description",
					"channels": [{"id": 1, "title": "Channel One", "slug": "channel-one"}],
					"genres": [{"id": 2, "name": "Drama"}],
					"subgenres": [{"id": 3, "name": "Melodrama"}],
					"images": [{"id": 4, "linkType": "Poster", "presets": [{"name": "Small", "link": "https://example.com/poster.jpg"}]}]
				},
				"episodesFilter": {
					"data": [
						{
							"id": 100,
							"title": "Episode One",
							"number": 1,
							"season": {"number": 1},
							"description": "Episode description",
							"createdAt": "2020-01-01T00:00:00Z",
							"airDate": "2020-01-02T00:00:00Z",
							"publicationDate": "2020-01-03T00:00:00Z",
							"audio": {"duration": 1800, "publicId": 555},
							"images": [{"id": 5, "linkType": "SplashScreen", "presets": [{"name": "Small", "link": "https://example.com/splash.jpg"}]}]
						}
					]
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 10, 10)
	require.NoError(t, err)

	airDate, _ := time.Parse(time.RFC3339, "2020-01-02T00:00:00Z")
	airDateVal := SmotrimTime(airDate)
	want := &BrandEpisodes{
		Brand: &Brand{
			ID:          10,
			Title:       "Brand Title",
			Description: "Brand description",
			Channels:    []Channel{{ID: 1, Title: "Channel One", Slug: "channel-one"}},
			Genres:      []Genre{{ID: 2, Name: "Drama"}},
			Subgenres:   []Subgenre{{ID: 3, Name: "Melodrama"}},
			Images: []Image{{
				ID:       4,
				LinkType: "Poster",
				Presets:  []ImagePreset{{Name: "Small", Link: "https://example.com/poster.jpg"}},
			}},
		},
		Episodes: []*Episode{
			{
				ID:              100,
				Title:           "Episode One",
				Number:          1,
				Season:          &Season{Number: 1},
				Description:     "Episode description",
				CreatedAt:       "2020-01-01T00:00:00Z",
				AirDate:         &airDateVal,
				PublicationDate: "2020-01-03T00:00:00Z",
				Audio:           &Audio{Duration: 1800, PublicId: 555},
				Images: []Image{{
					ID:       5,
					LinkType: "SplashScreen",
					Presets:  []ImagePreset{{Name: "Small", Link: "https://example.com/splash.jpg"}},
				}},
			},
		},
	}
	assert.Equal(t, want, got)

	// Verify the request Client.Do actually built for this call.
	var sentReq gqlRequest
	require.NoError(t, json.Unmarshal(capturedBody, &sentReq))
	assert.Equal(t, "BrandEpisodes", sentReq.OperationName)
	assert.Equal(t, brandEpisodesQuery, sentReq.Query)
	assert.JSONEq(t,
		`{"brandId":10,"page":1,"first":10,"airDateFrom":"2000-01-01T00:00:00Z","order":"DESC"}`,
		string(sentReq.Variables),
	)
}

func TestClient_BrandEpisodes_EmptyEpisodeList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"brand": {"id": 10, "title": "Brand Title"},
				"episodesFilter": {"data": []}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 10, 1)
	require.NoError(t, err)

	require.NotNil(t, got)
	assert.Equal(t, &Brand{ID: 10, Title: "Brand Title"}, got.Brand)
	assert.Empty(t, got.Episodes)
}

func TestClient_BrandEpisodes_BrandIsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": {
				"brand": null,
				"episodesFilter": {"data": []}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 999, 1)
	require.NoError(t, err)

	require.NotNil(t, got)
	assert.Nil(t, got.Brand)
	assert.Empty(t, got.Episodes)
}

func TestClient_BrandEpisodes_PropagatesGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"brand not found"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Nil(t, got)

	var gqlErr *Error
	require.True(t, errors.As(err, &gqlErr))
	assert.Equal(t, []string{"brand not found"}, gqlErr.Messages)
}

func TestClient_BrandEpisodes_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_BrandEpisodes_PropagatesTransportError(t *testing.T) {
	// Close the server before making the request, so the dial itself
	// fails - a real transport-level error, no fake Transport needed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClient_BrandEpisodes_PropagatesInvalidJSONEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.BrandEpisodes(context.Background(), 1, 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decode response envelope")
}
