package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/dreamsxin/go-kit/endpoint"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	client         HTTPClient
	req            EncodeRequestFunc
	dec            DecodeResponseFunc
	before         []RequestFunc  /* 发出请求前，改变 context */
	after          []ResponseFunc /* 成功返回后执行，改变 context */
	finalizer      []ResponseFunc /* 不管是否成功，都将执行 */
	bufferedStream bool
}

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

type ClientOption func(*Client)

func SetClient(client HTTPClient) ClientOption {
	return func(c *Client) { c.client = client }
}

func ClientBefore(before ...RequestFunc) ClientOption {
	return func(c *Client) { c.before = append(c.before, before...) }
}

func ClientAfter(after ...ResponseFunc) ClientOption {
	return func(c *Client) { c.after = append(c.after, after...) }
}

func ClientFinalizer(f ...ResponseFunc) ClientOption {
	return func(s *Client) { s.finalizer = append(s.finalizer, f...) }
}

// 设置 body 读取方式为缓存流的方式，需自行关闭和清空
func BufferedStream(buffered bool) ClientOption {
	return func(c *Client) { c.bufferedStream = buffered }
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
				for _, f := range c.finalizer {
					ctx = f(ctx, resp, err)
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
