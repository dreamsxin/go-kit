package orderservice

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/dreamsxin/go-kit/transport/http/server"
	idl "example.com/gen_idl_extend_append"
	genendpoint "example.com/gen_idl_extend_append/endpoint/orderservice"
)

// NewHTTPHandler returns the generated HTTP handler set.
func NewHTTPHandler(endpoints genendpoint.OrderServiceEndpoints) http.Handler {
	m := http.NewServeMux()
	registerHTTPServeMuxRoutes(m, endpoints)
	return m
}

// RegisterHTTPRoutes binds the generated HTTP routes onto a gorilla/mux router.
func RegisterHTTPRoutes(router *mux.Router, endpoints genendpoint.OrderServiceEndpoints, prefix string) {

	// POST /placeorder
	router.Handle(routePath(prefix, "/placeorder"), server.NewStrictJSONEndpoint[idl.PlaceOrderRequest](
		endpoints.PlaceOrderEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	)).Methods("POST")

}

func registerHTTPServeMuxRoutes(m *http.ServeMux, endpoints genendpoint.OrderServiceEndpoints) {

	m.Handle("POST /placeorder", server.NewStrictJSONEndpoint[idl.PlaceOrderRequest](
		endpoints.PlaceOrderEndpoint,
		server.DefaultMaxJSONBodyBytes,
		server.ServerErrorEncoder(server.JSONErrorEncoder),
	))

}

func routePath(prefix, route string) string {
	if prefix == "" {
		return route
	}
	return prefix + route
}


// decodePlaceOrderRequest uses the generated strict JSON decode path.
//
// @Summary      PlaceOrder
// @Description  PlaceOrder microservice endpoint
// @Tags         OrderService
// @Accept       json
// @Produce      json
// @Param        request  body      idl.PlaceOrderRequest  true  "PlaceOrder request"
// @Success      200      {object}  idl.PlaceOrderResponse
// @Failure      400      {object}  server.ErrorResponse
// @Failure      500      {object}  server.ErrorResponse
// @Router       /placeorder [post]
func decodePlaceOrderRequest(_ context.Context, r *http.Request) (any, error) {
	var req idl.PlaceOrderRequest
	if err := server.DecodeJSONBody(r, &req, server.StrictJSONDecodeOptions(server.DefaultMaxJSONBodyBytes)); err != nil {
		return nil, server.JSONDecodeError{Err: err}
	}
	return req, nil
}

func encodePlaceOrderResponse(ctx context.Context, w http.ResponseWriter, response any) error {
	return json.NewEncoder(w).Encode(response)
}

