package orderservice

import (
	"context"
	"time"

	"github.com/dreamsxin/go-kit/endpoint"
	kitlog "github.com/dreamsxin/go-kit/log"
	idl "example.com/gen_idl_extend_append"
	svc "example.com/gen_idl_extend_append/service/orderservice"
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
	CBEnabled:          true,
	CBFailureThreshold: 5,
	CBTimeout:          60 * time.Second,
	RLEnabled:          true,
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


// RetryMiddleware is intended for client-side endpoint usage.
func RetryMiddleware(maxRetries int, backoff time.Duration) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request any) (response any, err error) {
			for i := 0; i < maxRetries; i++ {
				response, err = next(ctx, request)
				if err == nil {
					return response, nil
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff * time.Duration(i+1)):
				}
			}
			return nil, err
		}
	}
}
