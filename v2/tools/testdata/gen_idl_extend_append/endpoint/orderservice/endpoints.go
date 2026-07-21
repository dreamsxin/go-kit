package orderservice

import (
	"context"
	"time"

	idl "example.com/gen_idl_extend_append"
	svc "example.com/gen_idl_extend_append/service/orderservice"
	"github.com/dreamsxin/go-kit/v2/endpoint"
	kitlog "github.com/dreamsxin/go-kit/v2/log"
)

// OrderServiceEndpoints groups the generated endpoints.
type OrderServiceEndpoints struct {
	PlaceOrderEndpoint endpoint.Endpoint
}

type MiddlewareConfig struct {
	CBEnabled          bool
	CBFailureThreshold uint32
	CBTimeout          time.Duration
	RLEnabled          bool
	RLRps              float64
	Timeout            time.Duration
}

var DefaultMiddlewareConfig = MiddlewareConfig{
	CBEnabled:          false,
	CBFailureThreshold: 5,
	CBTimeout:          60 * time.Second,
	RLEnabled:          false,
	RLRps:              100,
	Timeout:            30 * time.Second,
}

func MakeServerEndpoints(s svc.OrderService, logger *kitlog.Logger) OrderServiceEndpoints {
	return MakeServerEndpointsWithConfig(s, logger, DefaultMiddlewareConfig)
}

func MakeServerEndpointsWithConfig(
	s svc.OrderService,
	logger *kitlog.Logger,
	cfg MiddlewareConfig,
) OrderServiceEndpoints {
	build := func(ep endpoint.Endpoint, name string) endpoint.Endpoint {
		ep = applyGeneratedMiddleware(ep, logger, cfg, name)
		return applyCustomMiddleware(ep, logger, cfg, name)
	}
	return OrderServiceEndpoints{
		PlaceOrderEndpoint: build(MakePlaceOrderEndpoint(s), "PlaceOrder"),
	}
}

func MakePlaceOrderEndpoint(s svc.OrderService) endpoint.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		req := request.(idl.PlaceOrderRequest)
		resp, err := s.PlaceOrder(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

func (e OrderServiceEndpoints) PlaceOrder(ctx context.Context, req idl.PlaceOrderRequest) (idl.PlaceOrderResponse, error) {
	resp, err := e.PlaceOrderEndpoint(ctx, req)
	if err != nil {
		return idl.PlaceOrderResponse{}, err
	}
	return resp.(idl.PlaceOrderResponse), nil
}
