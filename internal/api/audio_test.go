package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validAudioJSON is a trimmed version of a real API response: it keeps every
// field the Audio struct cares about (plus a couple of unrelated ones, to
// make sure unknown fields are ignored) so tests stay focused and readable.
const validAudioJSON = `{
  "status": "OK",
  "data": {
    "id": 1316241,
    "publicId": 2883598,
    "duration": 586,
    "shareLink": "https://smotrim.ru/brand/67390#playing_audio=2883598",
    "streams": {
      "mp3": "https://podcast.smotrim.ru/audio/001/316/241.mp3?entity=audio&id=1316241",
      "http": null
    },
    "unknownField": "should be ignored"
  }
}`

func TestClient_Audio_Success(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validAudioJSON))
	}))
	defer server.Close()

	c := &apiClient{BaseURL: server.URL}
	audio, err := c.Audio(context.Background(), 2883598)

	require.NoError(t, err)
	require.NotNil(t, audio)
	assert.Equal(t, 1316241, audio.Id)
	assert.Equal(t, 2883598, audio.PublicId)
	assert.Equal(t, int64(586), audio.Duration)
	assert.Equal(t, "https://smotrim.ru/brand/67390#playing_audio=2883598", audio.ShareLink)
	assert.Equal(t, "https://podcast.smotrim.ru/audio/001/316/241.mp3?entity=audio&id=1316241", audio.Streams.Mp3)
	assert.Equal(t, "/audio/2883598", gotPath)
}

func TestClient_Audio_UsesDefaultBaseURLWhenUnset(t *testing.T) {
	c := &apiClient{}
	got, err := c.audioURL(2883598)

	require.NoError(t, err)
	assert.Equal(t, "https://player-api.smotrim.ru/api/v1/audio/2883598", got)
}

func TestClient_Audio_UsesConfiguredHTTPClient(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validAudioJSON))
	}))
	defer server.Close()

	c := &apiClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := c.Audio(context.Background(), 1)

	require.NoError(t, err)
	assert.True(t, called)
}

func TestClient_Audio_InvalidBaseURLReturnsErrorInsteadOfPanicking(t *testing.T) {
	c := &apiClient{BaseURL: "://not-a-valid-url"}

	require.NotPanics(t, func() {
		_, err := c.Audio(context.Background(), 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build url")
	})
}

func TestClient_Audio_NonOKStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"ERROR"}`))
	}))
	defer server.Close()

	c := &apiClient{BaseURL: server.URL}
	audio, err := c.Audio(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "ERROR")
}

func TestClient_Audio_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	c := &apiClient{BaseURL: server.URL}
	audio, err := c.Audio(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.Contains(t, err.Error(), "decode response envelope")
}

func TestClient_Audio_MissingDataField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"OK","data":null}`))
	}))
	defer server.Close()

	c := &apiClient{BaseURL: server.URL}
	audio, err := c.Audio(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.Contains(t, err.Error(), "no audio data")
}

func TestClient_Audio_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validAudioJSON))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &apiClient{BaseURL: server.URL}
	audio, err := c.Audio(ctx, 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.Contains(t, err.Error(), "do request")
}

// erroringRoundTripper simulates a transport that fails to read the response
// body, e.g. because the connection was interrupted mid-stream.
type erroringRoundTripper struct{}

func (erroringRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&brokenReader{}),
		Header:     make(http.Header),
	}, nil
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

func TestClient_Audio_ReadBodyError(t *testing.T) {
	c := &apiClient{
		BaseURL:    "http://example.invalid",
		HTTPClient: &http.Client{Transport: erroringRoundTripper{}},
	}

	audio, err := c.Audio(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.Contains(t, err.Error(), "read response")
}

// failingDoRoundTripper simulates a transport-level failure (e.g. DNS
// resolution failure, connection refused) before any response is received.
type failingDoRoundTripper struct{}

func (failingDoRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

func TestClient_Audio_DoRequestError(t *testing.T) {
	c := &apiClient{
		BaseURL:    "http://example.invalid",
		HTTPClient: &http.Client{Transport: failingDoRoundTripper{}},
	}

	audio, err := c.Audio(context.Background(), 1)

	require.Error(t, err)
	assert.Nil(t, audio)
	assert.True(t, strings.Contains(err.Error(), "do request"))
}

func TestClient_baseURL_DefaultsAndOverrides(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		c := &apiClient{}
		assert.Equal(t, baseURL, c.baseURL())
	})

	t.Run("override", func(t *testing.T) {
		c := &apiClient{BaseURL: "https://example.com/"}
		assert.Equal(t, "https://example.com/", c.baseURL())
	})
}

func TestClient_httpClient_DefaultsAndOverrides(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		c := &apiClient{}
		assert.Same(t, http.DefaultClient, c.httpClient())
	})

	t.Run("override", func(t *testing.T) {
		custom := &http.Client{}
		c := &apiClient{HTTPClient: custom}
		assert.Same(t, custom, c.httpClient())
	})
}
