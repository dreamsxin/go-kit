package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dreamsxin/go-kit/transport"
)

// JSONErrorEncoder is a transport.ErrorEncoder that always writes a JSON
// error body.  It inspects the error for optional interfaces:
//
//   - interfaces.StatusCoder  → uses that HTTP status code (default 500)
//   - interfaces.Headerer     → merges those headers into the response
//
// The response body is:  {"error": "<message>"}
//
// Use it with ServerErrorEncoder:
//
//	server.NewServer(ep, dec, enc,
//	    server.ServerErrorEncoder(server.JSONErrorEncoder),
//	)
//
// Or with NewJSONServer:
//
//	server.NewJSONServer[Req](handler,
//	    server.ServerErrorEncoder(server.JSONErrorEncoder),
//	)
var JSONErrorEncoder transport.ErrorEncoder = func(_ context.Context, err error, w http.ResponseWriter) {
	type statusCoder interface{ StatusCode() int }
	type headerer interface{ Headers() http.Header }

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if h, ok := err.(headerer); ok {
		for k, vals := range h.Headers() {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
	}

	code := http.StatusInternalServerError
	if sc, ok := err.(statusCoder); ok {
		code = sc.StatusCode()
	}
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, err.Error())
}
