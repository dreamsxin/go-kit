package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/endpoint"
)

// MaxStatusErrorBodyBytes is how much of a non-2xx response body HTTPStatusError
// keeps. The body is diagnostic, not a payload: it is held in memory for the
// lifetime of the error, and an upstream that answers a failure with megabytes of
// HTML must not be able to make that this service's problem.
const MaxStatusErrorBodyBytes int64 = 64 << 10

// DefaultMaxJSONResponseBytes bounds successful JSON responses decoded by
// NewJSONClient. Callers with a larger, intentional contract can use
// NewJSONClientWithMaxResponseBodyBytes.
const DefaultMaxJSONResponseBytes int64 = 4 << 20

// ErrResponseBodyTooLarge indicates that a successful response exceeded the
// configured JSON response limit.
var ErrResponseBodyTooLarge = errors.New("http client: response body too large")

// ResponseBodyTooLargeError reports the configured response body limit.
type ResponseBodyTooLargeError struct {
	Limit int64
}

func (e *ResponseBodyTooLargeError) Error() string {
	return fmt.Sprintf("%v (limit %d bytes)", ErrResponseBodyTooLarge, e.Limit)
}

func (e *ResponseBodyTooLargeError) Unwrap() error { return ErrResponseBodyTooLarge }

// HTTPStatusError reports a non-2xx response returned by NewJSONClient.
type HTTPStatusError struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte

	// Truncated reports that the upstream body was longer than
	// MaxStatusErrorBodyBytes and Body holds only its first bytes.
	//
	// It exists so a reader can tell a cut-off body from a malformed one. Body
	// is what a human reads while debugging, and half a JSON document looks
	// exactly like an upstream that answered garbage; ErrorCode also returns ""
	// for a truncated body, which is indistinguishable from an upstream that
	// sent no code.
	Truncated bool
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	body := strings.TrimSpace(string(e.Body))
	if body == "" {
		return fmt.Sprintf("http client: unexpected status %s", e.Status)
	}
	if e.Truncated {
		return fmt.Sprintf(
			"http client: unexpected status %s: %s (body truncated at %d bytes)",
			e.Status, body, MaxStatusErrorBodyBytes,
		)
	}
	return fmt.Sprintf("http client: unexpected status %s: %s", e.Status, body)
}

// PublicMessage reports the message a server may forward to its own clients:
// the upstream status, never the upstream body.
//
// Error() keeps the body for logs, but a server that returns this error from an
// endpoint would otherwise leak it: the built-in error encoders fall back to
// err.Error() below 500, so an upstream 404 body would land verbatim in the
// downstream response. Stating a public message is what takes that fallback out
// of the picture.
func (e *HTTPStatusError) PublicMessage() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("upstream request failed with status %s", e.Status)
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
	return NewJSONClientWithMaxResponseBodyBytes[Resp](method, rawURL, DefaultMaxJSONResponseBytes, options...)
}

// NewJSONClientWithMaxResponseBodyBytes creates a JSON client with an
// explicit successful response body limit.
func NewJSONClientWithMaxResponseBodyBytes[Resp any](method, rawURL string, maxResponseBodyBytes int64, options ...ClientOption) (endpoint.Endpoint, error) {
	if maxResponseBodyBytes <= 0 || maxResponseBodyBytes == math.MaxInt64 {
		return nil, fmt.Errorf("NewJSONClient: max response body bytes must be between 1 and %d", int64(math.MaxInt64-1))
	}
	tgt, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("NewJSONClient: invalid URL %q: %w", rawURL, err)
	}
	dec := DecodeJSONResponseWithMaxBodyBytes[Resp](maxResponseBodyBytes)
	encoder := EncodeJSONRequest
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		encoder = EncodeQueryRequest
	}
	return NewClient(method, tgt, encoder, dec, options...).Endpoint(), nil
}

// DecodeJSONResponse is the default JSON response decoder used by
// NewJSONClient. It rejects non-2xx responses with HTTPStatusError and caps
// successful bodies at DefaultMaxJSONResponseBytes. Use it as a building
// block when composing a custom client with NewExplicitClient, or wrap it to
// change only part of the behavior.
func DecodeJSONResponse[Resp any](_ context.Context, r *http.Response) (any, error) {
	return decodeJSONResponse[Resp](r, DefaultMaxJSONResponseBytes)
}

// DecodeJSONResponseWithMaxBodyBytes is DecodeJSONResponse with an explicit
// successful-body limit.
func DecodeJSONResponseWithMaxBodyBytes[Resp any](maxResponseBodyBytes int64) DecodeResponseFunc {
	return func(_ context.Context, r *http.Response) (any, error) {
		return decodeJSONResponse[Resp](r, maxResponseBodyBytes)
	}
}

func decodeJSONResponse[Resp any](r *http.Response, maxResponseBodyBytes int64) (any, error) {
	if r.StatusCode < http.StatusOK || r.StatusCode >= http.StatusMultipleChoices {
		return nil, newHTTPStatusError(r)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxResponseBodyBytes {
		return nil, &ResponseBodyTooLargeError{Limit: maxResponseBodyBytes}
	}
	var resp Resp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func newHTTPStatusError(r *http.Response) error {
	// One byte past the limit distinguishes a body that exactly fills it from
	// one that was cut short.
	body, _ := io.ReadAll(io.LimitReader(r.Body, MaxStatusErrorBodyBytes+1))
	truncated := int64(len(body)) > MaxStatusErrorBodyBytes
	if truncated {
		body = body[:MaxStatusErrorBodyBytes]
	}
	return &HTTPStatusError{
		StatusCode: r.StatusCode,
		Status:     r.Status,
		Header:     r.Header.Clone(),
		Body:       body,
		Truncated:  truncated,
	}
}

// NewJSONClientWithTimeout creates a JSON client endpoint wrapped with a
// context timeout. It is a convenience shorthand for:
//
//	ep, _ := NewJSONClient[Resp](method, rawURL, options...)
//	ep = endpoint.NewBuilder(ep).WithTimeout(timeout).Build()
//
// For retry with service discovery, use sd/client.NewEndpoint instead.
//
// Example:
//
//	ep, err := client.NewJSONClientWithTimeout[UserResp](
//	    http.MethodGet, "http://localhost:8080/users/1",
//	    2*time.Second,
//	)
func NewJSONClientWithTimeout[Resp any](
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
