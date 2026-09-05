package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// BenchmarkJSONRoundTrip measures the whole server path for one request:
// intercepting writer, decode, endpoint, encode.

type benchRequest struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type benchResponse struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func BenchmarkJSONRoundTrip(b *testing.B) {
	handler := server.NewTypedJSONServer(func(_ context.Context, req benchRequest) (benchResponse, error) {
		return benchResponse{ID: req.ID, Username: req.Username}, nil
	})
	body := `{"id":7,"username":"alice","email":"alice@example.com"}`

	b.ReportAllocs()
	for b.Loop() {
		request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// BenchmarkJSONRoundTripWithAccessLog adds the protocol-fact recorder, so the
// difference against BenchmarkJSONRoundTrip is what observability costs per
// request.
func BenchmarkJSONRoundTripWithAccessLog(b *testing.B) {
	inner := server.NewTypedJSONServer(func(_ context.Context, req benchRequest) (benchResponse, error) {
		return benchResponse{ID: req.ID, Username: req.Username}, nil
	})
	handler := server.AccessLogMiddleware(discardLogger())(inner)
	body := `{"id":7,"username":"alice","email":"alice@example.com"}`

	b.ReportAllocs()
	for b.Loop() {
		request := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			b.Fatalf("status = %d", recorder.Code)
		}
	}
}
