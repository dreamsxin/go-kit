package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

const maxStatusErrorBody = 64 << 10

// HTTPStatusError reports a non-2xx response returned by NewJSONClient.
type HTTPStatusError struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	body := strings.TrimSpace(string(e.Body))
	if body == "" {
		return fmt.Sprintf("http client: unexpected status %s", e.Status)
	}
	return fmt.Sprintf("http client: unexpected status %s: %s", e.Status, body)
}

// Retryable reports whether the status is generally safe to retry.
func (e *HTTPStatusError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode == http.StatusRequestTimeout ||
		e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode >= http.StatusInternalServerError
}

// NewJSONClient creates an HTTP client endpoint that sends JSON requests and
// decodes JSON responses into values of type Resp.
//
// method is the HTTP verb (GET, POST, …).
// rawURL is the full target URL string (e.g. "http://localhost:8080/users").
//
// Example:
//
//	type CreateReq  struct { Name string `json:"name"` }
//	type CreateResp struct { ID   uint   `json:"id"`   }
//
//	ep, err := client.NewJSONClient[CreateResp](
//	    http.MethodPost, "http://localhost:8080/users",
//	)
//	resp, err := ep(ctx, CreateReq{Name: "alice"})
//	user := resp.(CreateResp)
func NewJSONClient[Resp any](method, rawURL string, options ...ClientOption) (endpoint.Endpoint, error) {
	tgt, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("NewJSONClient: invalid URL %q: %w", rawURL, err)
	}
	dec := func(_ context.Context, r *http.Response) (any, error) {
		if r.StatusCode < http.StatusOK || r.StatusCode >= http.StatusMultipleChoices {
			return nil, newHTTPStatusError(r)
		}
		var resp Resp
		if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
	return NewClient(method, tgt, EncodeJSONRequest, dec, options...).Endpoint(), nil
}

func newHTTPStatusError(r *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxStatusErrorBody))
	return &HTTPStatusError{
		StatusCode: r.StatusCode,
		Status:     r.Status,
		Header:     r.Header.Clone(),
		Body:       body,
	}
}

// NewJSONClientWithRetry creates a JSON client endpoint wrapped with a
// context timeout.  It is a convenience shorthand for:
//
//	ep, _ := NewJSONClient[Resp](method, rawURL, options...)
//	ep = endpoint.NewBuilder(ep).WithTimeout(timeout).Build()
//
// For full retry-with-balancer support, use sd.NewEndpoint instead.
//
// Example:
//
//	ep, err := client.NewJSONClientWithRetry[UserResp](
//	    http.MethodGet, "http://localhost:8080/users/1",
//	    2*time.Second,
//	)
func NewJSONClientWithRetry[Resp any](
	method, rawURL string,
	timeout time.Duration,
	options ...ClientOption,
) (endpoint.Endpoint, error) {
	base, err := NewJSONClient[Resp](method, rawURL, options...)
	if err != nil {
		return nil, err
	}
	return endpoint.NewBuilder(base).WithTimeout(timeout).Build(), nil
}
