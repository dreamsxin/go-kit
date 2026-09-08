package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// contentTypeClaimingError is the error that used to produce two Content-Types:
// the encoder set one, then merged this one in with Add.
type contentTypeClaimingError struct{}

func (contentTypeClaimingError) Error() string { return "teapot" }

func (contentTypeClaimingError) ErrorKindName() string {
	return string(apperror.KindInvalidArgument)
}

func (contentTypeClaimingError) Headers() http.Header {
	header := http.Header{}
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("X-Reason", "kept")
	return header
}

// TestErrorEncodersKeepOneContentType covers every built-in error encoder. A
// response with two Content-Types is one no client can interpret, and the
// encoder is the thing that chose the body, so it names the type — last, after
// everything else the error asked for is merged.
func TestErrorEncodersKeepOneContentType(t *testing.T) {
	encoders := map[string]httpserver.ErrorEncoder{
		"DefaultErrorEncoder":            httpserver.DefaultErrorEncoder,
		"JSONErrorEncoder":               httpserver.JSONErrorEncoder,
		"JSONErrorEncoderWithKindMapper": httpserver.JSONErrorEncoderWithKindMapper(func(apperror.Kind) int { return 0 }),
		"ProblemJSONErrorEncoder":        httpserver.ProblemJSONErrorEncoder(nil),
	}
	want := map[string]string{
		"DefaultErrorEncoder":            "text/plain; charset=utf-8",
		"JSONErrorEncoder":               "application/json; charset=utf-8",
		"JSONErrorEncoderWithKindMapper": "application/json; charset=utf-8",
		"ProblemJSONErrorEncoder":        httpserver.ProblemMediaType,
	}

	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			encode(context.Background(), contentTypeClaimingError{}, recorder)

			got := recorder.Result().Header.Values("Content-Type")
			if len(got) != 1 {
				t.Fatalf("Content-Type = %v, want exactly one value", got)
			}
			if got[0] != want[name] {
				t.Errorf("Content-Type = %q, want %q", got[0], want[name])
			}
			// The rest of what the error asked for still arrives.
			if reason := recorder.Result().Header.Get("X-Reason"); reason != "kept" {
				t.Errorf("X-Reason = %q, want the header the error reported", reason)
			}
		})
	}
}

// TestJSONErrorEncoderStatusIsUnaffectedByTheHeaderOrder guards the neighbour:
// applying headers before naming the Content-Type must not disturb the status a
// Retry-After-reporting error resolves to.
func TestJSONErrorEncoderStatusIsUnaffectedByTheHeaderOrder(t *testing.T) {
	recorder := httptest.NewRecorder()
	httpserver.JSONErrorEncoder(context.Background(), contentTypeClaimingError{}, recorder)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
