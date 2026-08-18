package kit_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/dreamsxin/go-kit/v2/kit"
)

// ExampleHandleJSONTyped registers a fully typed JSON handler on a Service.
// Service implements http.Handler, so the service can be served by
// Service.Run in production or by any net/http server in tests.
func ExampleHandleJSONTyped() {
	type greetRequest struct {
		Name string `json:"name"`
	}
	type greetResponse struct {
		Message string `json:"message"`
	}

	svc, err := kit.New(":8080")
	if err != nil {
		fmt.Println("kit.New:", err)
		return
	}

	kit.HandleJSONTyped(svc, "/greet",
		func(_ context.Context, req greetRequest) (greetResponse, error) {
			return greetResponse{Message: "Hello, " + req.Name + "!"}, nil
		})

	server := httptest.NewServer(svc)
	defer server.Close()

	resp, err := http.Post(server.URL+"/greet",
		"application/json", strings.NewReader(`{"name":"go-kit"}`))
	if err != nil {
		fmt.Println("post:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(resp.StatusCode, string(body))

	// Output:
	// 200 {"message":"Hello, go-kit!"}
}
