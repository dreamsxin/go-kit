package orderservice

import (
	"context"
	"encoding/json"
	idl "example.com/gen_idl_extend_append"
	genendpoint "example.com/gen_idl_extend_append/endpoint/orderservice"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
	"net/http"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.OrderServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a standard ServeMux.
func RegisterHTTPRoutes(router *http.ServeMux, endpoints genendpoint.OrderServiceEndpoints, prefix string) {

	// POST /placeorder
	router.Handle("POST "+routePath(prefix, "/placeorder"), server.NewServer(
		endpoints.PlaceOrderEndpoint,
		decodePlaceOrderRequest,
		encodePlaceOrderResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.OrderServiceEndpoints) {

	m.Handle("POST /placeorder", server.NewServer(
		endpoints.PlaceOrderEndpoint,
		decodePlaceOrderRequest,
		encodePlaceOrderResponse,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}

// decodePlaceOrderRequest uses the generated method-aware decode path.
func decodePlaceOrderRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.PlaceOrderRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	if err := transporthttp.DecodePathRequest(r, &req); err != nil {
		return nil, err
	}
	return req, nil
}

func encodePlaceOrderResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}
