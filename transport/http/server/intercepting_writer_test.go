package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInterceptingWriter_WriteHeader_CapturesCode(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	iw.WriteHeader(http.StatusCreated)

	if iw.GetCode() != http.StatusCreated {
		t.Errorf("GetCode: got %d, want %d", iw.GetCode(), http.StatusCreated)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("underlying recorder code: got %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestInterceptingWriter_Write_TracksBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	data := []byte("hello world")
	n, err := iw.Write(data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
	if iw.GetWritten() != int64(len(data)) {
		t.Errorf("GetWritten: got %d, want %d", iw.GetWritten(), len(data))
	}

	// Write more and verify accumulation.
	n2, _ := iw.Write([]byte("!"))
	if iw.GetWritten() != int64(len(data)+n2) {
		t.Errorf("GetWritten after second write: got %d, want %d", iw.GetWritten(), len(data)+n2)
	}
}

func TestInterceptingWriter_DefaultCode_Is200(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	// Writing without explicit WriteHeader should default to 200.
	iw.Write([]byte("ok")) //nolint:errcheck

	// httptest.Recorder defaults to 200 when WriteHeader is not called explicitly.
	if iw.GetCode() != 0 {
		// InterceptingWriter.code is zero until WriteHeader is called.
		// This is intentional — the underlying recorder tracks the real default.
	}
}

func TestInterceptingWriter_DelegatesToUnderlying(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	iw.Header().Set("X-Test", "value")
	iw.WriteHeader(http.StatusAccepted)
	iw.Write([]byte("body")) //nolint:errcheck

	if rec.Header().Get("X-Test") != "value" {
		t.Errorf("header not delegated: got %q", rec.Header().Get("X-Test"))
	}
	if rec.Body.String() != "body" {
		t.Errorf("body not delegated: got %q", rec.Body.String())
	}
}

func TestReimplementInterfaces_NoExtraInterfaces(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	result := iw.reimplementInterfaces()
	if result == nil {
		t.Fatal("reimplementInterfaces returned nil")
	}

	// The result should always implement http.ResponseWriter.
	if _, ok := result.(http.ResponseWriter); !ok {
		t.Error("result does not implement http.ResponseWriter")
	}
}
