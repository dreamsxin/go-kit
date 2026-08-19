package server_test

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// buildMultipartBody builds a multipart/form-data body with one text field
// and one file part.
func buildMultipartBody(t *testing.T, fieldName, fieldValue, fileName, fileContent string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField(fieldName, fieldValue); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("upload", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(fileContent)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func multipartRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	body, contentType := buildMultipartBody(t, "note", "hello", "greeting.txt", "file contents")
	req := httptest.NewRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", contentType)
	return req
}

func TestParseMultipartForm_ReadsFieldsAndFiles(t *testing.T) {
	req := multipartRequest(t, "/upload")

	form, err := server.ParseMultipartForm(req, server.MultipartLimits{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := form.Value["note"][0]; got != "hello" {
		t.Errorf("field value: got %q, want hello", got)
	}

	fh := form.File["upload"][0]
	if fh.Filename != "greeting.txt" {
		t.Errorf("filename: got %q", fh.Filename)
	}
	file, err := fh.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, _ := io.ReadAll(file)
	if string(content) != "file contents" {
		t.Errorf("file content: got %q", content)
	}

	// A second parse reuses the form already attached to the request.
	again, err := server.ParseMultipartForm(req, server.MultipartLimits{})
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if again != form {
		t.Error("second parse should reuse the parsed form")
	}
}

func TestParseMultipartForm_BodyTooLargeIs413(t *testing.T) {
	req := multipartRequest(t, "/upload")

	_, err := server.ParseMultipartForm(req, server.MultipartLimits{MaxBodyBytes: 16})
	if !errors.Is(err, server.ErrMultipartBodyTooLarge) {
		t.Fatalf("error: want ErrMultipartBodyTooLarge, got %v", err)
	}

	var sc interface{ StatusCode() int }
	if !errors.As(err, &sc) || sc.StatusCode() != http.StatusRequestEntityTooLarge {
		t.Errorf("error should classify as 413, got %+v", err)
	}
}

func TestParseMultipartForm_FileTooLargeIs413(t *testing.T) {
	req := multipartRequest(t, "/upload")

	_, err := server.ParseMultipartForm(req, server.MultipartLimits{MaxFileBytes: 4})
	if !errors.Is(err, server.ErrMultipartFileTooLarge) {
		t.Fatalf("error: want ErrMultipartFileTooLarge, got %v", err)
	}
	var sc interface{ StatusCode() int }
	if !errors.As(err, &sc) || sc.StatusCode() != http.StatusRequestEntityTooLarge {
		t.Errorf("error should classify as 413, got %+v", err)
	}
}

func TestParseMultipartForm_NonMultipartIs415(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("plain"))
	req.Header.Set("Content-Type", "application/json")

	_, err := server.ParseMultipartForm(req, server.MultipartLimits{})
	if err == nil {
		t.Fatal("expected error for non-multipart content type")
	}
	var sc interface{ StatusCode() int }
	if !errors.As(err, &sc) || sc.StatusCode() != http.StatusUnsupportedMediaType {
		t.Errorf("error should classify as 415, got %+v", err)
	}
}

func TestWriteAttachment_SetsHeadersAndStreamsContent(t *testing.T) {
	rec := httptest.NewRecorder()
	content := "attachment payload"

	if err := server.WriteAttachment(rec, "report.txt", int64(len(content)), strings.NewReader(content)); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type: got %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "18" {
		t.Errorf("Content-Length: got %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="report.txt"` &&
		!strings.HasPrefix(got, "attachment; filename") {
		t.Errorf("Content-Disposition: got %q", got)
	}
	if rec.Body.String() != content {
		t.Errorf("body: got %q", rec.Body.String())
	}
}

func TestWriteAttachment_SanitizesHostileFilename(t *testing.T) {
	rec := httptest.NewRecorder()

	if err := server.WriteAttachment(rec, `evil"quote.txt`, 0, strings.NewReader("x")); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	disposition := rec.Header().Get("Content-Disposition")
	if strings.ContainsAny(disposition, "\r\n") {
		t.Errorf("Content-Disposition must not contain line breaks: %q", disposition)
	}
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("Content-Disposition: got %q", disposition)
	}
}

func TestWriteAttachment_UnknownExtensionIsOctetStream(t *testing.T) {
	rec := httptest.NewRecorder()

	if err := server.WriteAttachment(rec, "data.unknownext", 0, strings.NewReader("x")); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type: got %q, want application/octet-stream", got)
	}
}
