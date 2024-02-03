package client

import (
	"context"
	"net/http"
	"net/url"
)

// 发出请求前可以进行额外的工作，将信息放入 context
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
