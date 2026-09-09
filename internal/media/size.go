package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HttpFileSizer fetches file sizes for multiple URLs concurrently
type HttpFileSizer struct {
	client         *http.Client
	maxConcurrency int
}

// NewHttpFileSizer creates a new sizer with sensible defaults
func NewHttpFileSizer(maxConcurrency int) *HttpFileSizer {
	if maxConcurrency <= 0 {
		maxConcurrency = 10
	}
	transport := &http.Transport{
		MaxIdleConns:        maxConcurrency * 2,
		MaxIdleConnsPerHost: maxConcurrency,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // works like Accept-Encoding: identity
	}

	return &HttpFileSizer{
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
		maxConcurrency: maxConcurrency,
	}
}

// Sizes returns a map of URL to file size
// URLs that fail to fetch or have unknown sizes are omitted from the map.
func (s *HttpFileSizer) Sizes(ctx context.Context, urls []string) map[string]int64 {
	results := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Semaphore to limit concurrency and prevent bans
	sem := make(chan struct{}, s.maxConcurrency)

	for _, url := range urls {
		// Check if context is already canceled before spawning a new goroutine
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore (blocks if limit is reached)

		go func(u string) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			size, err := s.size(ctx, u)
			if err == nil && size > 0 {
				mu.Lock()
				results[u] = size
				mu.Unlock()
			}
		}(url)
	}

	wg.Wait()
	return results
}

// size tries HEAD, then falls back to Range
func (s *HttpFileSizer) size(ctx context.Context, url string) (int64, error) {
	size, err := s.tryHeadRequest(ctx, url)
	if err == nil && size > 0 {
		return size, nil
	}
	return s.tryRangeRequest(ctx, url)
}

// tryHeadRequest executes a HEAD request
func (s *HttpFileSizer) tryHeadRequest(ctx context.Context, url string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("failed to create HEAD request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("head request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("tryHeadRequest: failed to close response body", slog.Any("error", err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("head request failed with status %d", resp.StatusCode)
	}

	return resp.ContentLength, nil
}

// tryRangeRequest executes a GET request with Range: bytes=0-0
func (s *HttpFileSizer) tryRangeRequest(ctx context.Context, url string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, fmt.Errorf("failed to create Range request: %w", err)
	}

	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("range request failed: %w", err)
	}

	// CRITICAL: Close body immediately to prevent downloading large files
	// if the server ignores the Range header.
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("tryRangeRequest: failed to close response body", slog.Any("error", err))
		}
	}()

	if resp.StatusCode == http.StatusPartialContent {
		contentRange := resp.Header.Get("Content-Range")
		if contentRange == "" {
			return 0, errors.New("status 206 but missing Content-Range header")
		}
		return parseContentRange(contentRange)
	}

	if resp.StatusCode == http.StatusOK {
		if resp.ContentLength > 0 {
			return resp.ContentLength, nil
		}
		return 0, errors.New("server ignored Range and did not provide Content-Length")
	}

	return 0, fmt.Errorf("unexpected response status: %d", resp.StatusCode)
}

// parseContentRange extracts the total size from a Content-Range header.
func parseContentRange(header string) (int64, error) {
	parts := strings.Split(header, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid Content-Range format: %s", header)
	}

	sizeStr := parts[1]
	if sizeStr == "*" {
		return 0, errors.New("file size is unknown to the server (*)")
	}

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse size from Content-Range: %w", err)
	}

	return size, nil
}
