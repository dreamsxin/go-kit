package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// HTTPClient is the subset of *http.Client this transport relies on, so tests
// and lightweight callers can substitute a stub.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client adapts one HTTP request/response pair to an endpoint.Endpoint.
type Client struct {
	client         HTTPClient
	req            EncodeRequestFunc
	dec            DecodeResponseFunc
	before         []RequestFunc
	after          []ResponseFunc
	finalizer      []FinalizerFunc // Always runs, regardless of success or failure.
	bufferedStream bool
}

// NewClient constructs an HTTP client using method/target-based request creation.
func NewClient(method string, tgt *url.URL, enc EncodeRequestFunc, dec DecodeResponseFunc, options ...ClientOption) *Client {
	if tgt == nil || enc == nil {
		panic("essential parameters cannot be nil")
	}
	return NewExplicitClient(makeCreateRequestFunc(method, tgt, enc), dec, options...)
}

// NewExplicitClient constructs an HTTP client from explicit request/response functions.
//
// Stable: httpclient.traceparent-by-default — every outbound call carries the
// context's W3C traceparent without an opt-in hook, matching the gRPC client;
// a context without a trace leaves the header unset, and a ClientBefore hook
// still wins because it runs after the built-in one.
// Covered by: TestClientInjectsTraceparentByDefault, TestClientWithoutTraceLeavesHeaderUnset
func NewExplicitClient(req EncodeRequestFunc, dec DecodeResponseFunc, options ...ClientOption) *Client {
	if req == nil || dec == nil {
		panic("essential parameters cannot be nil")
	}
	c := &Client{
		client: defaultClient,
		req:    req,
		dec:    dec,
		// Trace context propagates by default, matching the gRPC client: a
		// client that had to opt in would silently break every trace at the
		// first HTTP hop somebody forgot. ClientBefore hooks run after this
		// one and can overwrite the header.
		before: []RequestFunc{transporthttp.InjectTraceparent},
	}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c Client) Endpoint() endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		ctx, cancel := context.WithCancel(ctx)

		var (
			resp *http.Response
			err  error
		)
		if c.finalizer != nil {
			defer func() {
				// Guard against resp being nil when request construction or
				// the underlying HTTP call fails before a response is received.
				if resp != nil {
					ctx = context.WithValue(ctx, transporthttp.ContextKeyResponseHeaders, resp.Header)
					ctx = context.WithValue(ctx, transporthttp.ContextKeyResponseSize, resp.ContentLength)
				}
				for _, f := range c.finalizer {
					f(ctx, err)
				}
			}()
		}

		req, err := c.req(ctx, nil, request)
		if err != nil {
			cancel()
			return nil, err
		}

		for _, f := range c.before {
			ctx = f(ctx, req)
		}

		resp, err = c.client.Do(req.WithContext(ctx))
		if err != nil {
			cancel()
			return nil, err
		}

		if c.bufferedStream {
			resp.Body = bodyWithCancel{ReadCloser: resp.Body, cancel: cancel}
		} else {
			defer resp.Body.Close()
			defer cancel()
		}

		for _, f := range c.after {
			ctx = f(ctx, resp, nil)
		}

		response, err := c.dec(ctx, resp)
		if err != nil {
			if c.bufferedStream {
				_ = resp.Body.Close()
			}
			return nil, err
		}

		return response, nil
	}
}

type bodyWithCancel struct {
	io.ReadCloser

	cancel context.CancelFunc
}

func (bwc bodyWithCancel) Close() error {
	err := bwc.ReadCloser.Close()
	bwc.cancel()
	return err
}
