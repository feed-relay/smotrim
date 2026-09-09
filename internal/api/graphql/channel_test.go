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

func TestClient_Channel_Success(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b

		_, _ = w.Write([]byte(`{"data":{"channel":{"id":42,"title":"Some Title","slug":"some-slug"}}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Channel(context.Background(), 42)
	require.NoError(t, err)
	assert.Equal(t, &Channel{ID: 42, Title: "Some Title", Slug: "some-slug"}, got)

	// Verify the request Client.Do actually built for this call.
	var sentReq gqlRequest
	require.NoError(t, json.Unmarshal(capturedBody, &sentReq))
	assert.Equal(t, "Channel", sentReq.OperationName)
	assert.Equal(t, queryChannel, sentReq.Query)
	assert.JSONEq(t, `{"id":42}`, string(sentReq.Variables))
}

func TestClient_Channel_NotFound_ReturnsNilChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"channel":null}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Channel(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClient_Channel_PropagatesGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"channel not found"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Channel(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)

	var gqlErr *Error
	require.True(t, errors.As(err, &gqlErr))
	assert.Equal(t, []string{"channel not found"}, gqlErr.Messages)
}

func TestClient_Channel_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.Channel(context.Background(), 1)
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_ChannelBySlug_Success(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b

		_, _ = w.Write([]byte(`{"data":{"channel":{"id":7,"title":"Slug Title","slug":"my-slug"}}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.ChannelBySlug(context.Background(), "my-slug")
	require.NoError(t, err)
	assert.Equal(t, &Channel{ID: 7, Title: "Slug Title", Slug: "my-slug"}, got)

	var sentReq gqlRequest
	require.NoError(t, json.Unmarshal(capturedBody, &sentReq))
	assert.Equal(t, "ChannelBySlug", sentReq.OperationName)
	assert.Equal(t, queryChannelBySlug, sentReq.Query)
	assert.JSONEq(t, `{"slug":"my-slug"}`, string(sentReq.Variables))
}

func TestClient_ChannelBySlug_NotFound_ReturnsNilChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"channel":null}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.ChannelBySlug(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClient_ChannelBySlug_PropagatesGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"slug not found"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	got, err := c.ChannelBySlug(context.Background(), "missing")
	require.Error(t, err)
	assert.Nil(t, got)

	var gqlErr *Error
	require.True(t, errors.As(err, &gqlErr))
	assert.Equal(t, []string{"slug not found"}, gqlErr.Messages)
}

func TestClient_ChannelBySlug_SpecialCharactersAreEscapedNotInjected(t *testing.T) {
	var capturedVars json.RawMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var sentReq gqlRequest
		require.NoError(t, json.Unmarshal(b, &sentReq))
		capturedVars = sentReq.Variables

		_, _ = w.Write([]byte(`{"data":{"channel":null}}`))
	}))
	t.Cleanup(srv.Close)

	// A slug with a quote, a backslash, and an attempted key-injection,
	// all in one string.
	tricky := `some"slug\with\backslashes","injected":"value`

	c := &Client{Endpoint: srv.URL}
	_, err := c.ChannelBySlug(context.Background(), tricky)
	require.NoError(t, err)

	// It must round-trip as exactly one "slug" value, byte for byte -
	// no broken JSON, and no extra key sneaked into the object.
	var parsedVars map[string]any
	require.NoError(t, json.Unmarshal(capturedVars, &parsedVars))
	assert.Len(t, parsedVars, 1)
	assert.Equal(t, tricky, parsedVars["slug"])
}
