package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// 将用户的数据类型转为 http.Request 对象
type EncodeRequestFunc func(context.Context, *http.Request /*可以为 nil */, interface{} /*用户的数据类型*/) (*http.Request, error)

// 将 http.Response 对象转为用户的自定义类型
type DecodeResponseFunc func(context.Context, *http.Response) (response interface{} /*用户的自定义类型*/, err error)

// 用于接口类型判断
type StatusCoder interface {
	StatusCode() int
}

type Headerer interface {
	Headers() http.Header
}

// 将用户的请求数据转为 http.Request 请求对象，并将类型设置为 json
func EncodeJSONRequest(c context.Context, req *http.Request, request interface{}) (*http.Request, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := request.(Headerer); ok {
		for k := range headerer.Headers() {
			req.Header.Set(k, headerer.Headers().Get(k))
		}
	}
	var b bytes.Buffer
	req.Body = io.NopCloser(&b)
	return req, json.NewEncoder(&b).Encode(request)
}
