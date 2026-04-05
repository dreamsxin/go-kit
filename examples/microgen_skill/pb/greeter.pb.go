package pb

import (
	"context"
)

type HelloRequest struct {
	Name string `json:"name"`
	Tags []string `json:"tags"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

type Empty struct{}

type StatusResponse struct {
	Alive   bool   `json:"alive"`
	Version string `json:"version"`
}

// GreeterServer is the server API for Greeter service.
type GreeterServer interface {
	SayHello(context.Context, *HelloRequest) (*HelloResponse, error)
	GetStatus(context.Context, *Empty) (*StatusResponse, error)
}

// Mock gRPC registration
func RegisterGreeterServer(s any, srv GreeterServer) {
	// Mock registration
}
