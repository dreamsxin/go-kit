package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/dreamsxin/go-kit/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/transport/http"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	client         HTTPClient
	req            EncodeRequestFunc
	dec            DecodeResponseFunc
	before         []RequestFunc   /* 发出请求前，改变 context */
	after          []ResponseFunc  /* 成功返回后执行，改变 context */
	finalizer      []FinalizerFunc /* 不管是否成功，都将执行 */
	bufferedStream bool
}

// 创建 http 请求客户端
func NewClient(method string, tgt *url.URL, enc EncodeRequestFunc, dec DecodeResponseFunc, options ...ClientOption) *Client {
	return NewExplicitClient(makeCreateRequestFunc(method, tgt, enc), dec, options...)
}

func NewExplicitClient(req EncodeRequestFunc, dec DecodeResponseFunc, options ...ClientOption) *Client {
	c := &Client{
		client: http.DefaultClient,
		req:    req,
		dec:    dec,
	}
	for _, option := range options {
		option(c)
	}
	return c
}

func (c Client) Endpoint() endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		ctx, cancel := context.WithCancel(ctx)

		var (
			resp *http.Response
			err  error
		)
		if c.finalizer != nil {
			defer func() {
				ctx = context.WithValue(ctx, transporthttp.ContextKeyResponseHeaders, resp.Header)
				ctx = context.WithValue(ctx, transporthttp.ContextKeyResponseSize, resp.ContentLength)
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
	bwc.ReadCloser.Close()
	bwc.cancel()
	return nil
}
