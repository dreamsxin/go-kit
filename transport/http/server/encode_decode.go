package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/transport/http/interfaces"
)

// 将 http.Request 对象转为用户的数据类型
type DecodeRequestFunc func(context.Context, *http.Request) (request interface{}, err error)

// 用于不需要解码的请求
func NopRequestDecoder(ctx context.Context, r *http.Request) (interface{}, error) {
	return nil, nil
}

// 将用户发送数据传递给 http.Response 对象
type EncodeResponseFunc func(context.Context, http.ResponseWriter, interface{}) error

func NopResponseEncoder(context.Context, http.ResponseWriter, interface{}) error {
	return nil
}

// 将用户发送的数据转为 json 并写入 http.ResponseWriter
func EncodeJSONResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := response.(interfaces.Headerer); ok {
		for k, values := range headerer.Headers() {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}
	}
	code := http.StatusOK
	if sc, ok := response.(interfaces.StatusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)
	if code == http.StatusNoContent {
		return nil
	}
	return json.NewEncoder(w).Encode(response)
}
