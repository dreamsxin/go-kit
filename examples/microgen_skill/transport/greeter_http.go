package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "github.com/dreamsxin/go-kit/examples/microgen_skill/pb"
	genendpoint "github.com/dreamsxin/go-kit/examples/microgen_skill/endpoint"
)

// NewHTTPHandler 返回服务的 HTTP Handler
func NewHTTPHandler(endpoints genendpoint.GreeterEndpoints) http.Handler {
	m := http.NewServeMux()


	// POST /sayhello
	m.Handle("POST /sayhello", server.NewJSONEndpoint[idl.HelloRequest](
		endpoints.SayHelloEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

	// GET /getstatus
	m.Handle("GET /getstatus", server.NewJSONEndpoint[idl.Empty](
		endpoints.GetStatusEndpoint,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))


	return m
}


// DecodeSayHelloRequest 默认使用 NewJSONServer 的自动解码。
// 如果需要自定义解码逻辑（如从 URL 参数提取），请在此实现。
func DecodeSayHelloRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.HelloRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

// DecodeGetStatusRequest 默认使用 NewJSONServer 的自动解码。
// 如果需要自定义解码逻辑（如从 URL 参数提取），请在此实现。
func DecodeGetStatusRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.Empty
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return req, nil
}

