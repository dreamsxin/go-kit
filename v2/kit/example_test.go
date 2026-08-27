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

// ExampleHandleJSONTyped registers a fully typed JSON handler on an HTTP
// component. The component implements http.Handler, so it can be attached to
// a Host and run in production or served by any net/http server in tests.
func ExampleHandleJSONTyped() {
	type greetRequest struct {
		Name string `json:"name"`
	}
	type greetResponse struct {
		Message string `json:"message"`
	}

	httpComponent, err := kit.NewHTTP(":8080")
	if err != nil {
		fmt.Println("kit.NewHTTP:", err)
		return
	}

	kit.HandleJSONTyped(httpComponent, "/greet",
		func(_ context.Context, req greetRequest) (greetResponse, error) {
			return greetResponse{Message: "Hello, " + req.Name + "!"}, nil
		})

	server := httptest.NewServer(httpComponent)
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
