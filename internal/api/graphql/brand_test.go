package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// Brand(ctx, id)
// -----------------------------------------------------------------------

func TestClient_Brand_Success(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b

		_, _ = w.Write([]byte(`{
			"data": {
				"brand": {
					"id": 55,
					"title": "Brand Title",
					"description": "Brand description",
					"channels": [{"id": 1, "title": "Channel One", "slug": "channel-one"}],
					"images": [{
						"id": 4,
						"linkType": "Poster",
						"presets": [{"name": "Small", "link": "https://example.com/poster.jpg"}]
					}],
					"genres": [{"id": 2, "name": "Drama"}],
					"subgenres": [{"id": 3, "name": "Melodrama"}]
				}
			}
		}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 55)
	require.NoError(t, err)

	want := &Brand{
		ID:          55,
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
	}
	assert.Equal(t, want, got)

	// Verify the request Client.Do actually built for this call.
	var sentReq gqlRequest
	require.NoError(t, json.Unmarshal(capturedBody, &sentReq))
	assert.Equal(t, "Brand", sentReq.OperationName)
	assert.Equal(t, queryBrand, sentReq.Query)
	assert.JSONEq(t, `{"id":55}`, string(sentReq.Variables))
}

func TestClient_Brand_NotFound_ReturnsNilBrand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"brand":null}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClient_Brand_PropagatesGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"brand not found"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)

	var gqlErr *Error
	require.True(t, errors.As(err, &gqlErr))
	assert.Equal(t, []string{"brand not found"}, gqlErr.Messages)
}

func TestClient_Brand_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_Brand_PropagatesTransportError(t *testing.T) {
	// Close the server before making the request, so the dial itself
	// fails - a real transport-level error, no fake Transport needed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClient_Brand_PropagatesInvalidJSONEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decode response envelope")
}

func TestClient_Brand_PropagatesResultDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "brand" is a string, but Brand wants to decode it into a struct.
		_, _ = w.Write([]byte(`{"data":{"brand":"this-is-not-an-object"}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Brand(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decode data")
}
