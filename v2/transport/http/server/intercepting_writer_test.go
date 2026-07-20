package server

import (
	"bytes"
	"io"
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

	if iw.GetCode() != http.StatusOK {
		t.Fatalf("GetCode: got %d, want %d", iw.GetCode(), http.StatusOK)
	}
}

func TestInterceptingWriter_IgnoresRepeatedWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	iw := &InterceptingWriter{ResponseWriter: rec}

	iw.WriteHeader(http.StatusCreated)
	iw.WriteHeader(http.StatusInternalServerError)

	if iw.GetCode() != http.StatusCreated {
		t.Fatalf("GetCode: got %d, want %d", iw.GetCode(), http.StatusCreated)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("underlying code: got %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestInterceptingWriter_ReadFromTracksBytes(t *testing.T) {
	rec := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
	iw := &InterceptingWriter{ResponseWriter: rec}

	n, err := iw.ReadFrom(bytes.NewBufferString("streamed"))
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if n != int64(len("streamed")) || iw.GetWritten() != n {
		t.Fatalf("written = %d, ReadFrom = %d", iw.GetWritten(), n)
	}
	if _, ok := iw.reimplementInterfaces().(io.ReaderFrom); !ok {
		t.Fatal("ReaderFrom capability was not restored")
	}
}

type readerFromRecorder struct {
	*httptest.ResponseRecorder
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	return io.Copy(r.ResponseRecorder, src)
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
