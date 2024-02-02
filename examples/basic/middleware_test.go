package basic

import (
	"context"
	"fmt"
	"testing"

	"github.com/dreamsxin/go-kit/endpoint"
)

var (
	ctx = context.Background()
	req = struct{}{}
)

func annotate(s string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, request interface{}) (interface{}, error) {
			fmt.Println(s, "pre")
			defer fmt.Println(s, "post")
			return next(ctx, request)
		}
	}
}

func myEndpoint(context.Context, interface{}) (interface{}, error) {
	fmt.Println("my endpoint!")
	return struct{}{}, nil
}

// go test -v -count=1 -run TestExampleChain .\middleware_test.go
func TestExampleChain(t *testing.T) {
	e := endpoint.Chain(
		annotate("first"),
		annotate("second"),
		annotate("third"),
	)(myEndpoint)

	if _, err := e(ctx, req); err != nil {
		t.Fatal(err)
	}

	// Output:
	// first pre
	// second pre
	// third pre
	// my endpoint!
	// third post
	// second post
	// first post
}
