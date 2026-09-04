package server_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// An over-limit body is not a malformed body. Both the JSON decoder and the raw
// codec must answer 413, the status ParseMultipartForm already used and the one
// RawBodyCodecWithMaxBytes documents.
func TestOverLimitBodyIs413(t *testing.T) {
	body := strings.Repeat("a", 64)

	t.Run("json", func(t *testing.T) {
		decode := server.DecodeJSONRequestWithOptions[map[string]any](server.JSONDecodeOptions{MaxBodyBytes: 8})
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"k":"`+body+`"}`))

		_, err := decode(context.Background(), request)
		if err == nil {
			t.Fatal("decode should reject the over-limit body")
		}
		assertStatus(t, err, http.StatusRequestEntityTooLarge)
	})

	t.Run("raw", func(t *testing.T) {
		decode, _ := server.RawBodyCodecWithMaxBytes(
			func(b []byte) (any, error) { return string(b), nil },
			func(any) ([]byte, error) { return nil, nil },
			"application/octet-stream", 8,
		)
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		_, err := decode(context.Background(), request)
		if err == nil {
			t.Fatal("decode should reject the over-limit body")
		}
		assertStatus(t, err, http.StatusRequestEntityTooLarge)
	})
}

// A malformed body is still 400: the 413 must be specific to the size limit.
func TestMalformedJSONStays400(t *testing.T) {
	decode := server.DecodeJSONRequest[map[string]any]()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))

	_, err := decode(context.Background(), request)
	if err == nil {
		t.Fatal("decode should reject malformed JSON")
	}
	assertStatus(t, err, http.StatusBadRequest)
}

// WriteHeader panics on a status outside 100-999, so a response type must not be
// able to take the process down through StatusCoder.
func TestSuccessEncodersIgnoreAnInvalidStatus(t *testing.T) {
	encoders := map[string]server.EncodeResponseFunc{
		"json": server.EncodeJSONResponse,
		"wrap": server.WrapJSONResponse(func(response any) any { return response }),
	}
	rawEncode := func() server.EncodeResponseFunc {
		_, encode := server.RawBodyCodec(
			func(b []byte) (any, error) { return string(b), nil },
			func(any) ([]byte, error) { return []byte("body"), nil },
			"application/octet-stream",
		)
		return encode
	}
	encoders["raw"] = rawEncode()

	for name, encode := range encoders {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if err := encode(context.Background(), recorder, zeroStatusResponse{}); err != nil {
				t.Fatalf("encode: %v", err)
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", recorder.Code)
			}
		})
	}
}

type zeroStatusResponse struct{}

func (zeroStatusResponse) StatusCode() int { return 0 }

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	server.JSONErrorEncoder(context.Background(), err, recorder)
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, want, bytes.TrimSpace(recorder.Body.Bytes()))
	}
}
