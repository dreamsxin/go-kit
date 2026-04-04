package client

import (
	"context"
	"net/http"
	"net/url"
)

// RequestFunc is called before the HTTP request is sent.  It receives the
// current context and the outgoing *http.Request, and returns a (possibly
// enriched) context.  Use it to inject headers, auth tokens, etc.
type RequestFunc func(context.Context, *http.Request) context.Context

func makeCreateRequestFunc(method string, target *url.URL, enc EncodeRequestFunc) EncodeRequestFunc {
	return func(ctx context.Context, req *http.Request, request interface{}) (*http.Request, error) {
		if req == nil {
			_req, err := http.NewRequest(method, target.String(), nil)
			if err != nil {
				return nil, err
			}
			req = _req
		}
		req, err := enc(ctx, req, request)
		if err != nil {
			return nil, err
		}

		return req, nil
	}
}
