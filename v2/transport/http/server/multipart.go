package server

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
)

// Default caps for multipart uploads. Requests that need different bounds
// pass explicit MultipartLimits.
const (
	DefaultMaxMultipartBodyBytes   int64 = 32 << 20 // 32 MiB total body
	DefaultMaxMultipartMemoryBytes int64 = 8 << 20  // 8 MiB kept in memory
)

var (
	// ErrMultipartBodyTooLarge indicates the total multipart request body
	// exceeded MaxBodyBytes.
	ErrMultipartBodyTooLarge = errors.New("multipart request body too large")
	// ErrMultipartFileTooLarge indicates a single uploaded file exceeded
	// MaxFileBytes.
	ErrMultipartFileTooLarge = errors.New("uploaded file too large")
)

// MultipartLimits bounds multipart/form-data uploads. Zero fields select the
// package defaults; a negative MaxFileBytes disables the per-file check.
type MultipartLimits struct {
	// MaxBodyBytes caps the total request body. Zero selects
	// DefaultMaxMultipartBodyBytes.
	//
	// It is also the bound on work. Parts above MaxMemoryBytes spill to
	// temporary files while the body is being read, so this — not MaxFileBytes
	// — is what limits how much a request can make this process write to disk.
	MaxBodyBytes int64
	// MaxMemoryBytes is the in-memory threshold before file parts spill to
	// temporary files. Zero selects DefaultMaxMultipartMemoryBytes.
	MaxMemoryBytes int64
	// MaxFileBytes caps each individual file. Zero or negative disables the
	// per-file check; MaxBodyBytes still applies.
	//
	// It bounds the answer, not the work: a file's size is known only once the
	// part has been read, so an over-limit file is refused after it was written
	// and then removed. Set MaxBodyBytes to the disk cost you are willing to
	// pay, and MaxFileBytes to the size your application accepts.
	MaxFileBytes int64
}

// ParseMultipartForm parses a multipart/form-data request within the given
// limits and returns the parsed form. The form is owned by the request and
// reused on later calls; call RemoveAll to release spilled temporary files.
//
// Errors classify as client errors: bodies and files beyond the limits map
// to 413, malformed or non-multipart requests to 415/400, so they render
// correctly through JSONErrorEncoder.
//
// Stable: http.multipart-limits — an over-limit body is 413 request_too_large, an over-limit file 413 request_too_large.file, and a non-multipart request 415.
// Covered by: TestParseMultipartForm_BodyTooLargeIs413, TestParseMultipartForm_FileTooLargeIs413, TestParseMultipartForm_NonMultipartIs415
//
// Stable: http.multipart-file-refusal-leaves-nothing-behind — a request refused for an over-limit file leaves no temporary file and no parsed form behind.
// Covered by: TestParseMultipartFormRemovesTheSpilledFileItRefuses
func ParseMultipartForm(r *http.Request, limits MultipartLimits) (*multipart.Form, error) {
	if r.MultipartForm != nil {
		return r.MultipartForm, nil
	}

	maxBody := limits.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = DefaultMaxMultipartBodyBytes
	}
	maxMemory := limits.MaxMemoryBytes
	if maxMemory <= 0 {
		maxMemory = DefaultMaxMultipartMemoryBytes
	}

	if r.ContentLength > maxBody {
		return nil, multipartError{err: ErrMultipartBodyTooLarge, code: "request_too_large", status: http.StatusRequestEntityTooLarge}
	}
	r.Body = &limitedMultipartBody{ReadCloser: r.Body, remaining: maxBody}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		switch {
		case errors.Is(err, ErrMultipartBodyTooLarge):
			return nil, multipartError{err: ErrMultipartBodyTooLarge, code: "request_too_large", status: http.StatusRequestEntityTooLarge}
		case errors.Is(err, http.ErrNotMultipart), errors.Is(err, http.ErrMissingBoundary):
			return nil, multipartError{err: err, code: "unsupported_media_type", status: http.StatusUnsupportedMediaType}
		default:
			return nil, multipartError{err: err, code: "bad_request.invalid_multipart", status: http.StatusBadRequest}
		}
	}

	if limits.MaxFileBytes > 0 {
		for _, headers := range r.MultipartForm.File {
			for _, fh := range headers {
				if fh.Size > limits.MaxFileBytes {
					_ = r.MultipartForm.RemoveAll()
					r.MultipartForm = nil
					return nil, multipartError{err: ErrMultipartFileTooLarge, code: "request_too_large.file", status: http.StatusRequestEntityTooLarge}
				}
			}
		}
	}
	return r.MultipartForm, nil
}

// multipartError classifies multipart failures for the error encoders.
type multipartError struct {
	err    error
	code   string
	status int
}

func (e multipartError) Error() string {
	if e.err == nil {
		return "invalid multipart request"
	}
	return e.err.Error()
}

func (e multipartError) Unwrap() error { return e.err }

func (e multipartError) StatusCode() int { return e.status }

func (e multipartError) ErrorCode() string { return e.code }

type limitedMultipartBody struct {
	io.ReadCloser
	remaining int64
}

func (r *limitedMultipartBody) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, ErrMultipartBodyTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.ReadCloser.Read(p)
	r.remaining -= int64(n)
	return n, err
}

// WriteAttachment streams content to the client as a file download. The
// filename is encoded safely (RFC 2231 for non-ASCII names), the content
// type is derived from the filename extension, and a known size becomes the
// Content-Length header.
//
// Stable: http.attachment-headers — a download carries Content-Disposition attachment with an escaped filename, a type from its extension, and Content-Length when the size is known.
// Covered by: TestWriteAttachment_SetsHeadersAndStreamsContent, TestWriteAttachment_SanitizesHostileFilename, TestWriteAttachment_UnknownExtensionIsOctetStream
func WriteAttachment(w http.ResponseWriter, filename string, size int64, content io.Reader) error {
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if disposition == "" {
		disposition = `attachment; filename="download"`
	}
	w.Header().Set("Content-Disposition", disposition)

	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, err := io.Copy(w, content)
	return err
}
