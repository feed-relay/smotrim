package graphql

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

//go:generate moq --out ./mocks/roundtripper_mock.go --pkg mocks --skip-ensure --with-resets -fmt goimports . RoundTripper

type RoundTripper interface {
	RoundTrip(*http.Request) (*http.Response, error)
}

// defaultEndpoint is used whenever Client.Endpoint is left empty.
const defaultEndpoint = "https://apis.smotrim.ru/graphql"

// Error is returned when the server responds 200 OK but with one or
// more entries in the top-level GraphQL "errors" array.
type Error struct {
	Messages []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("graphql error(s): %v", e.Messages)
}

// Client is a minimal GraphQL client for the smotrim.ru API.
//
// The zero value Client{} is usable: HTTPClient falls back to
// http.DefaultClient and Endpoint falls back to defaultEndpoint. Use
// NewClient for the common case, or set the fields directly (e.g. to
// point Endpoint at an httptest.Server in tests).
type Client struct {
	// HTTPClient performs the actual HTTP round trip. If nil,
	// http.DefaultClient is used.
	HTTPClient *http.Client

	// Endpoint overrides the GraphQL endpoint URL. If empty,
	// defaultEndpoint is used. Exposing this mainly lets tests point the
	// client at an httptest.Server instead of the real API.
	Endpoint string

	// AuthToken, when set, is sent as a Bearer token.
	AuthToken string
}

// NewClient returns a Client with sane defaults (http.DefaultClient and
// defaultEndpoint). Both can still be overridden on the returned value.
func NewClient() *Client {
	return &Client{HTTPClient: http.DefaultClient}
}

type gqlRequest struct {
	OperationName string          `json:"operationName"`
	Variables     json.RawMessage `json:"variables,omitempty"`
	Query         string          `json:"query"`
}

type gqlError struct {
	Message string `json:"message"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

func (c *Client) Do(ctx context.Context, operationName, query, vars string, result any) error {
	r, err := c.buildRequest(ctx, operationName, query, vars)
	if err != nil {
		return fmt.Errorf("building request failed: %w", err)
	}
	resp, err := c.httpClient().Do(r)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Error("failed to close response body", slog.Any("error", closeErr))
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, string(respBody))
	}

	var gr gqlResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return fmt.Errorf("decode response envelope: %w", err)
	}

	if len(gr.Errors) > 0 {
		msgs := make([]string, 0, len(gr.Errors))
		for _, e := range gr.Errors {
			msgs = append(msgs, e.Message)
		}
		return &Error{Messages: msgs}
	}

	if result != nil && len(gr.Data) > 0 {
		if err := json.Unmarshal(gr.Data, result); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}

	return nil
}

// httpClient returns c.HTTPClient, falling back to http.DefaultClient so
// a zero-value Client doesn't panic with a nil pointer dereference.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// endpoint returns c.Endpoint, falling back to defaultEndpoint.
func (c *Client) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return defaultEndpoint
}

func (c *Client) buildRequest(ctx context.Context, operationName, query, vars string) (*http.Request, error) {
	body, err := c.marshalBody(operationName, query, vars)
	if err != nil {
		return nil, err
	}

	reqURL, err := c.buildURL(operationName, body, []byte(vars))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	return req, nil
}

// marshalBody builds the compact, non-HTML-escaped JSON body of the
// GraphQL request.
func (c *Client) marshalBody(operationName, query, vars string) ([]byte, error) {
	reqBody := gqlRequest{
		OperationName: operationName,
		Variables:     json.RawMessage(vars),
		Query:         query,
	}
	return c.jsonMarshalCompactNoEscapeHTML(reqBody)
}

// buildURL appends the "page"/"body"/"vars" tracking query params (used
// by the upstream API, apparently for caching/analytics) to c.endpoint().
// Building it via url.Values keeps escaping correct even if Endpoint ever
// grows its own query string.
func (c *Client) buildURL(operationName string, body, vars []byte) (string, error) {
	u, err := url.Parse(c.endpoint())
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}

	q := u.Query()
	q.Set("page", operationName)
	q.Set("body", c.md5Hex(body))
	q.Set("vars", c.md5Hex(vars))
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/graphql-response+json, application/json")
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Origin", "https://smotrim.ru")
	req.Header.Set("Referer", "https://smotrim.ru/")
}

func (c *Client) jsonMarshalCompactNoEscapeHTML(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (c *Client) md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}
