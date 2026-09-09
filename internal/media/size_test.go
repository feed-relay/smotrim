package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPSizeFetcher_GetSizes(t *testing.T) {
	t.Run("Concurrent successful fetches", func(t *testing.T) {
		fetcher := NewHttpFileSizer(2)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		urls := []string{
			server.URL + "/file1.zip",
			server.URL + "/file2.zip",
			server.URL + "/file3.zip",
		}

		sizes := fetcher.Sizes(context.Background(), urls)

		require.Len(t, sizes, 3)
		for _, url := range urls {
			assert.Equal(t, int64(4096), sizes[url])
		}
	})

	t.Run("Mixed success and failure", func(t *testing.T) {
		fetcher := NewHttpFileSizer(2)
		successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
		}))
		defer successServer.Close()

		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer failServer.Close()

		urls := []string{
			successServer.URL + "/ok.txt",
			failServer.URL + "/notfound.txt",
			"http://invalid-domain-that-does-not-exist-12345.com/file.txt",
		}

		sizes := fetcher.Sizes(context.Background(), urls)

		// Only the successful URL should be in the map
		require.Len(t, sizes, 1)
		assert.Equal(t, int64(1024), sizes[successServer.URL+"/ok.txt"])
	})

	t.Run("Context cancellation stops processing", func(t *testing.T) {
		fetcher := NewHttpFileSizer(2)
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer slowServer.Close()

		urls := make([]string, 50)
		for i := range urls {
			urls[i] = slowServer.URL
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		sizes := fetcher.Sizes(ctx, urls)
		elapsed := time.Since(start)

		// Should fail fast and return empty/partial map
		assert.Less(t, elapsed.Seconds(), float64(1.0))
		assert.LessOrEqual(t, len(sizes), len(urls))
	})

	t.Run("Concurrency limit is respected", func(t *testing.T) {
		var activeRequests int
		var maxActiveRequests int
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			activeRequests++
			if activeRequests > maxActiveRequests {
				maxActiveRequests = activeRequests
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond) // Simulate network delay

			mu.Lock()
			activeRequests--
			mu.Unlock()

			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		urls := make([]string, 10)
		for i := range urls {
			urls[i] = server.URL
		}

		// Limit concurrency to 3
		limitedFetcher := NewHttpFileSizer(3)
		_ = limitedFetcher.Sizes(context.Background(), urls)

		mu.Lock()
		defer mu.Unlock()

		// Max active requests should not exceed the limit (3)
		assert.LessOrEqual(t, maxActiveRequests, 3, "Concurrency limit was exceeded")
	})
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    int64
		wantErr bool
	}{
		{"Valid range", "bytes 0-1023/1024", 1024, false},
		{"Valid range with space", "bytes 0-0/500", 500, false},
		{"Invalid format", "invalid-header", 0, true},
		{"Missing slash", "bytes 0-1023", 0, true},
		{"Unknown size", "bytes 0-0/*", 0, true},
		{"Invalid size", "bytes 0-0/abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContentRange(tt.header)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
