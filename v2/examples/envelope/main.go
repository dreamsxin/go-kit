// Package main demonstrates transport-level response assembly: business
// handlers return pure domain types, and the {code, message, data} envelope
// plus its matching error format are defined once for the whole service
// through kit.WithJSONServerOptions.
//
// Concepts shown:
//   - kit.WithJSONServerOptions installs ServerResponseEncoder and
//     ServerErrorEncoder for every JSON route in one place
//   - business code never sees the envelope: handlers return HelloResponse
//     and classified apperror values
//   - apperror kinds drive the error status and the stable machine-readable
//     code, so clients can branch on codes instead of parsing prose
//   - per-route options can still override the service-wide assembly
//
// Run:
//
//	go run ./examples/envelope
//
// Test with curl:
//
//	# Success: 200 with the envelope around the domain response
//	curl -X POST http://localhost:8080/hello \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":"world"}'
//
//	# Validation failure: 400 with the same envelope and a stable code
//	curl -i -X POST http://localhost:8080/hello \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":""}'
//
//	# A route that opts out of the envelope with a per-route option
//	curl -X POST http://localhost:8080/raw \
//	     -H "Content-Type: application/json" \
//	     -d '{"name":"world"}'
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// ── Domain types (no envelope, no framework dependency) ──────────────────────

type HelloRequest struct {
	Name string `json:"name"`
}

type HelloResponse struct {
	Message string `json:"message"`
}

// ── Transport assembly: the envelope, defined once ───────────────────────────

// apiEnvelope is the wire format assembled by the transport layer.
type apiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// encodeAPIResponse wraps every successful response in the envelope.
func encodeAPIResponse(_ context.Context, w http.ResponseWriter, response any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(apiEnvelope{Code: 0, Message: "ok", Data: response})
}

// encodeAPIError renders classified application errors in the same envelope
// shape; unclassified errors stay opaque to clients.
func encodeAPIError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(apiEnvelope{Code: http.StatusInternalServerError, Message: "internal error"})
		return
	}

	status := http.StatusInternalServerError
	switch appErr.ErrorKind() {
	case apperror.KindInvalidArgument:
		status = http.StatusBadRequest
	case apperror.KindNotFound:
		status = http.StatusNotFound
	case apperror.KindUnauthenticated:
		status = http.StatusUnauthorized
	case apperror.KindPermissionDenied:
		status = http.StatusForbidden
	}

	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiEnvelope{Code: status, Message: appErr.PublicMessage()})
}

// ── Business logic ───────────────────────────────────────────────────────────

func hello(_ context.Context, req HelloRequest) (HelloResponse, error) {
	if req.Name == "" {
		return HelloResponse{}, apperror.New(
			apperror.KindInvalidArgument,
			"hello.name_required",
			"name is required",
		)
	}
	return HelloResponse{Message: "Hello, " + req.Name + "!"}, nil
}

// ── Wire-up ──────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	svc, err := kit.New(*httpAddr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
		// Response assembly lives here, once, at the transport boundary.
		kit.WithJSONServerOptions(
			server.ServerResponseEncoder(encodeAPIResponse),
			server.ServerErrorEncoder(encodeAPIError),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	kit.HandleJSONTyped(svc, "/hello", hello)

	// A route that opts out of the envelope: per-route options run after
	// the service-wide ones and take precedence.
	kit.HandleJSONTyped(svc, "/raw", hello,
		server.ServerResponseEncoder(server.EncodeJSONResponse),
	)

	log.Println("envelope example listening on", *httpAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
