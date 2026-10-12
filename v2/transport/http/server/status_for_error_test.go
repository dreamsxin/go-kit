package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

type statusCoded struct {
	err    error
	status int
}

func (e statusCoded) Error() string   { return e.err.Error() }
func (e statusCoded) StatusCode() int { return e.status }
func (e statusCoded) Unwrap() error   { return e.err }

// TestHTTPStatusForErrorMatchesTheEncoders pins the exported status helper to
// the encoders' actual behavior: for every error a request can carry, the
// helper's number is the status the wire saw.
func TestHTTPStatusForErrorMatchesTheEncoders(t *testing.T) {
	decodeErr := server.JSONDecodeError{Err: errors.New("cannot unmarshal object into []string")}

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"decode failure is 400", decodeErr, http.StatusBadRequest},
		{"classified kind", apperror.New(apperror.KindNotFound, "x", "nope"), http.StatusNotFound},
		{"status coder wins", statusCoded{errors.New("custom"), http.StatusTeapot}, http.StatusTeapot},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"canceled", context.Canceled, 499},
		{"unclassified", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		if got := server.HTTPStatusForError(tc.err); got != tc.want {
			t.Errorf("%s: HTTPStatusForError = %d, want %d", tc.name, got, tc.want)
		}
		h := server.NewServer(
			func(context.Context, any) (any, error) { return nil, tc.err },
			func(_ context.Context, _ *http.Request) (any, error) { return struct{}{}, nil },
			server.EncodeJSONResponse,
			server.ServerErrorEncoder(server.JSONErrorEncoder),
		)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`)))
		if rec.Code != tc.want {
			t.Errorf("%s: encoder wrote %d, helper says %d", tc.name, rec.Code, tc.want)
		}
	}
}
