package graphql

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/feed-relay/smotrim/internal/api/graphql/mocks"
)

// mustMD5Hex is an independent (test-local) re-implementation of the
// hex(md5(...)) computation, used to verify Client's own hashing without
// depending on its private helper.
func mustMD5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

// -----------------------------------------------------------------------
// Do(): happy path and request-construction details, via a real
// httptest.Server. This exercises the full net/http stack (real
// serialization, real headers) rather than a fake transport.
// -----------------------------------------------------------------------

func TestClient_Do_Success(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b

		_, _ = w.Write([]byte(`{"data":{"foo":"bar"}}`))
	}))
	t.Cleanup(srv.Close)

	// HTTPClient is deliberately left nil here to also exercise the
	// http.DefaultClient fallback.
	c := &Client{Endpoint: srv.URL, AuthToken: "test-token"}

	var res struct {
		Foo string `json:"foo"`
	}
	err := c.Do(context.Background(), "MyOp", "query MyOp { foo }", `{"id":1}`, &res)
	require.NoError(t, err)
	assert.Equal(t, "bar", res.Foo)

	require.NotNil(t, capturedReq)
	assert.Equal(t, http.MethodPost, capturedReq.Method)

	// Query params, including hash consistency with the actual body sent.
	q := capturedReq.URL.Query()
	assert.Equal(t, "MyOp", q.Get("page"))
	assert.Equal(t, mustMD5Hex([]byte(`{"id":1}`)), q.Get("vars"))
	assert.Equal(t, mustMD5Hex(capturedBody), q.Get("body"))

	// Headers.
	assert.Equal(t, "application/json", capturedReq.Header.Get("Content-Type"))
	assert.Equal(t, "application/graphql-response+json, application/json", capturedReq.Header.Get("Accept"))
	assert.Equal(t, "Bearer test-token", capturedReq.Header.Get("Authorization"))
	assert.Equal(t, "Mozilla/5.0", capturedReq.Header.Get("User-Agent"))
	assert.Equal(t, "https://smotrim.ru", capturedReq.Header.Get("Origin"))
	assert.Equal(t, "https://smotrim.ru/", capturedReq.Header.Get("Referer"))

	// Body content.
	var sentReq gqlRequest
	require.NoError(t, json.Unmarshal(capturedBody, &sentReq))
	assert.Equal(t, "MyOp", sentReq.OperationName)
	assert.Equal(t, "query MyOp { foo }", sentReq.Query)
	assert.JSONEq(t, `{"id":1}`, string(sentReq.Variables))
}

func TestClient_Do_NoAuthToken_NoAuthorizationHeader(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL} // AuthToken left empty on purpose
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.NoError(t, err)

	assert.Empty(t, authHeader)
}

func TestClient_Do_EmptyVariables_OmittedFromBody(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = b
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query Op { x }", "", nil)
	require.NoError(t, err)

	assert.NotContains(t, string(capturedBody), `"variables"`)
}

func TestClient_Do_OperationNameIsQueryEscaped(t *testing.T) {
	var capturedReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "weird op&name", "query", "", nil)
	require.NoError(t, err)

	// url.Values.Get decodes the query param automatically, proving the
	// value survived an escape/decode round trip intact.
	assert.Equal(t, "weird op&name", capturedReq.URL.Query().Get("page"))
}

// -----------------------------------------------------------------------
// GraphQL-level errors (HTTP 200, but an "errors" array is present)
// -----------------------------------------------------------------------

func TestClient_Do_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"field not found"},{"message":"unauthorized"}]}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.Error(t, err)

	var gqlErr *Error
	require.True(t, errors.As(err, &gqlErr))
	assert.Equal(t, []string{"field not found", "unauthorized"}, gqlErr.Messages)
	assert.Contains(t, err.Error(), "field not found")
}

// -----------------------------------------------------------------------
// HTTP-level failures
// -----------------------------------------------------------------------

func TestClient_Do_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom, server on fire"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "boom, server on fire")
}

func TestClient_Do_TransportError(t *testing.T) {
	// Close the server before making the request, so the dial itself
	// fails - a real transport-level error, no fake Transport needed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClient_Do_InvalidJSONEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response envelope")
}

func TestClient_Do_ResultDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "data" is a string, but the caller wants to decode it into a struct.
		_, _ = w.Write([]byte(`{"data":"this-is-not-an-object"}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	var res struct {
		Foo string `json:"foo"`
	}
	err := c.Do(context.Background(), "Op", "query", "", &res)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode data")
}

func TestClient_Do_BuildRequestFails_InvalidVariablesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server must not be called when request building fails")
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	// Malformed vars makes the outer JSON encoding of gqlRequest fail,
	// so buildRequest should error out before any HTTP call is attempted.
	err := c.Do(context.Background(), "Op", "query", "{not-valid-json", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "building request failed")
}

// -----------------------------------------------------------------------
// Edge cases around the "data"/"result" handling
// -----------------------------------------------------------------------

func TestClient_Do_NilResult_DataIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"foo":"bar"}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	assert.NoError(t, err)
}

func TestClient_Do_EmptyData_ResultUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Endpoint: srv.URL}
	res := struct{ Foo string }{Foo: "unchanged"}
	err := c.Do(context.Background(), "Op", "query", "", &res)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", res.Foo)
}

// -----------------------------------------------------------------------
// Failure modes a real server can't reliably produce: these still use
// the moq-generated RoundTripperMock to inject a broken response body.
// -----------------------------------------------------------------------

// brokenReader always fails, simulating a connection dropped mid-response.
type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) {
	return 0, errors.New("boom: connection reset")
}

func TestClient_Do_ReadResponseBodyError(t *testing.T) {
	mockRT := &mocks.RoundTripperMock{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(brokenReader{}),
			}, nil
		},
	}

	c := &Client{HTTPClient: &http.Client{Transport: mockRT}}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read response")

	require.Len(t, mockRT.RoundTripCalls(), 1)
}

// errOnCloseBody is a response body whose Close always fails, to exercise
// the deferred close-error branch in Do (which only logs, never returns).
type errOnCloseBody struct {
	io.Reader
}

func (errOnCloseBody) Close() error { return errors.New("close failed") }

func TestClient_Do_BodyCloseErrorIsSwallowed(t *testing.T) {
	mockRT := &mocks.RoundTripperMock{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			b, _ := json.Marshal(map[string]any{})
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errOnCloseBody{Reader: bytes.NewReader(b)},
			}, nil
		},
	}

	c := &Client{HTTPClient: &http.Client{Transport: mockRT}}
	err := c.Do(context.Background(), "Op", "query", "", nil)
	// Do must still succeed: the body-close error is only logged via
	// slog.Error in the deferred func, never propagated as a return value.
	assert.NoError(t, err)
}

// -----------------------------------------------------------------------
// Defaults: nil HTTPClient / empty Endpoint must not panic.
// -----------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient()
	assert.Same(t, http.DefaultClient, c.HTTPClient)
	assert.Empty(t, c.Endpoint)
	assert.Equal(t, defaultEndpoint, c.endpoint())
}

func TestClient_endpoint_FallsBackToDefault(t *testing.T) {
	c := &Client{}
	assert.Equal(t, defaultEndpoint, c.endpoint())

	c.Endpoint = "https://example.com/graphql"
	assert.Equal(t, "https://example.com/graphql", c.endpoint())
}

func TestClient_httpClient_FallsBackToDefault(t *testing.T) {
	c := &Client{}
	assert.Same(t, http.DefaultClient, c.httpClient())

	custom := &http.Client{}
	c.HTTPClient = custom
	assert.Same(t, custom, c.httpClient())
}

// -----------------------------------------------------------------------
// Unit tests for the small private helpers, exercised directly since this
// is a whitebox (same-package) test file.
// -----------------------------------------------------------------------

func TestClient_buildURL(t *testing.T) {
	c := &Client{Endpoint: "https://example.com/graphql"}

	body := []byte(`{"operationName":"Op"}`)
	vars := []byte(`{"id":1}`)

	got, err := c.buildURL("Op", body, vars)
	require.NoError(t, err)

	u, err := url.Parse(got)
	require.NoError(t, err)
	assert.Equal(t, "example.com", u.Host)
	assert.Equal(t, "/graphql", u.Path)

	q := u.Query()
	assert.Equal(t, "Op", q.Get("page"))
	assert.Equal(t, mustMD5Hex(body), q.Get("body"))
	assert.Equal(t, mustMD5Hex(vars), q.Get("vars"))
}

func TestClient_buildURL_InvalidEndpoint(t *testing.T) {
	c := &Client{Endpoint: "http://example.com/%zz"} // invalid percent-encoding
	_, err := c.buildURL("Op", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse endpoint")
}

func TestClient_jsonMarshalCompactNoEscapeHTML(t *testing.T) {
	c := &Client{}

	out, err := c.jsonMarshalCompactNoEscapeHTML(map[string]string{"a": "<b>&"})
	require.NoError(t, err)

	// HTML-special characters must survive un-escaped...
	assert.Equal(t, `{"a":"<b>&"}`, string(out))
	// ...and there must be no trailing newline (json.Encoder normally adds one).
	assert.False(t, bytes.HasSuffix(out, []byte("\n")))
}

func TestClient_md5Hex(t *testing.T) {
	c := &Client{}

	got := c.md5Hex([]byte("hello"))
	assert.Equal(t, mustMD5Hex([]byte("hello")), got)
	assert.Len(t, got, 32) // an md5 hex digest is always 32 chars
}

func TestClientRequest_WaitsForLimiter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"foo":"bar"}}`))
	}))
	t.Cleanup(srv.Close)

	limiter := &mocks.LimiterMock{
		WaitFunc: func(context.Context) error {
			return nil
		},
	}

	c := &Client{Endpoint: srv.URL, Limiter: limiter}
	err := c.Do(context.Background(), "Op", "query", "", nil)

	assert.NoError(t, err)
	if len(limiter.WaitCalls()) != 1 {
		t.Fatalf("expected 1 call, got %d", len(limiter.WaitCalls()))
	}
}

func TestClientRequest_LimiterError(t *testing.T) {
	expectedErr := errors.New("rate limit")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"foo":"bar"}}`))
	}))
	t.Cleanup(srv.Close)

	limiter := &mocks.LimiterMock{
		WaitFunc: func(context.Context) error {
			return expectedErr
		},
	}

	c := &Client{Endpoint: srv.URL, Limiter: limiter}
	err := c.Do(context.Background(), "Op", "query", "", nil)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}
