package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	"github.com/dreamsxin/go-kit/v2/transport"
)

// SSEStream writes Server-Sent Events to an HTTP response. The zero value is
// not usable; SSEServer creates one and passes it to the stream handler.
//
// Methods are not safe for concurrent use: event writes from multiple
// goroutines must be serialized by the caller.
type SSEStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// Data writes one unnamed data event. Multi-line data is written as one
// "data:" line per line, as required by the SSE format.
func (ss *SSEStream) Data(data string) error {
	return ss.writeEvent("", data)
}

// Event writes one named event.
func (ss *SSEStream) Event(name, data string) error {
	return ss.writeEvent(name, data)
}

// EventJSON marshals v as JSON and writes it as one named event.
func (ss *SSEStream) EventJSON(name string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ss.writeEvent(name, string(payload))
}

// Comment writes a comment line. Comments are ignored by clients and are the
// standard keep-alive heartbeat for streams behind proxies that time out
// idle connections.
//
// Text spanning several lines becomes one comment line per line, so a comment
// stays a comment: a blank line inside it would otherwise end the comment and
// let whatever followed be read as an event.
func (ss *SSEStream) Comment(text string) error {
	var b strings.Builder
	for _, line := range splitSSELines(text) {
		b.WriteString(": " + line + "\n")
	}
	if _, err := ss.w.Write([]byte(b.String())); err != nil {
		return err
	}
	ss.flusher.Flush()
	return nil
}

// Retry advises clients to wait the given milliseconds before reconnecting
// after a dropped connection.
func (ss *SSEStream) Retry(milliseconds int) error {
	_, err := fmt.Fprintf(ss.w, "retry: %d\n", milliseconds)
	if err != nil {
		return err
	}
	ss.flusher.Flush()
	return nil
}

// Stable: http.sse-framing — an event is "event:" then one "data:" line per input line, ended by a blank line; comments are ":" and reconnect advice "retry:".
// Covered by: TestSSEServer_MultiLineDataSplitsIntoDataLines, TestSSEServer_CommentAndRetryLines
//
// Stable: http.sse-line-terminators — CRLF, CR and LF each start a new data line, so a payload carrying any of them arrives whole rather than truncated at the first one a client recognises.
// Covered by: TestSSEStreamSplitsEveryLineTerminator
//
// Stable: http.sse-no-frame-injection — data and comment text cannot introduce a frame of their own, and an event name carrying a line terminator is an error rather than a frame.
// Covered by: TestSSEStreamDataCannotInjectAnEvent, TestSSEStreamCommentCannotInjectAnEvent, TestSSEStreamRejectsAnEventNameWithALineTerminator
func (ss *SSEStream) writeEvent(name, data string) error {
	// A field value cannot contain a line terminator, so a name that does is a
	// caller bug rather than data to reshape: stripping it would frame an event
	// under a name nobody asked for, and passing it through would let the name
	// end the field and inject whatever follows.
	if strings.ContainsAny(name, "\r\n") {
		return fmt.Errorf("server: SSE event name cannot contain a line terminator: %q", name)
	}
	var b strings.Builder
	if name != "" {
		b.WriteString("event: " + name + "\n")
	}
	for i, line := range splitSSELines(data) {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("data: " + line)
	}
	b.WriteString("\n\n")
	if _, err := ss.w.Write([]byte(b.String())); err != nil {
		return err
	}
	ss.flusher.Flush()
	return nil
}

// splitSSELines splits text the way an SSE client does.
//
// The specification ends a line on CRLF, CR, or LF. Splitting on LF alone left a
// bare CR inside a "data:" line, where a conforming client ends the field and
// reads the remainder as a field with no name — so the payload arrived silently
// truncated, and text after the CR could be read as another field entirely.
func splitSSELines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

// SSEStreamHandler runs one Server-Sent Events stream with the decoded
// request value. The request context is cancelled when the client
// disconnects. Errors returned after the first event was written cannot
// reach the client; the stream handler should emit a terminal event when
// clients need to learn about a failure.
type SSEStreamHandler func(ctx context.Context, request any, stream *SSEStream) error

// SSEServer adapts an SSEStreamHandler to http.Handler. It reuses the Server
// lifecycle hooks where they fit streaming:
//
//   - ServerBefore hooks run before the stream starts;
//   - the DecodeRequestFunc runs before any SSE headers are written, so decode
//     failures map to regular error responses through the ErrorEncoder;
//   - ServerErrorHandler observes decode failures and errors returned by the
//     stream handler after streaming began;
//   - ServerFinalizer hooks always run when the stream ends.
//
// ServerAfter and ServerResponseEncoder do not apply to streams and are
// ignored. One stream counts as one request for endpoint middleware composed
// around this handler: timeouts bound the total stream duration, so
// long-lived streams should avoid or relax global deadlines.
type SSEServer struct {
	cfg    Server // shared option surface: before, errorEncoder, finalizer, errorHandler
	stream SSEStreamHandler
}

// NewSSEServer constructs an SSE server for the given stream handler.
// stream and dec are required; passing nil panics, matching NewServer.
func NewSSEServer(stream SSEStreamHandler, dec DecodeRequestFunc, options ...ServerOption) *SSEServer {
	if stream == nil || dec == nil {
		panic("essential parameters cannot be nil")
	}
	s := &SSEServer{
		stream: stream,
		cfg: Server{
			dec:          dec,
			errorEncoder: DefaultErrorEncoder,
		},
	}
	for _, option := range options {
		option(&s.cfg)
	}
	if s.cfg.errorEncoder == nil {
		s.cfg.errorEncoder = DefaultErrorEncoder
	}
	if s.cfg.errorHandler == nil {
		s.cfg.errorHandler = transport.NopErrorHandler
	}
	return s
}

// NewSSEServerTyped constructs an SSE server with concrete request types,
// removing type assertions from the stream handler. The decode function maps
// the raw HTTP request to Req; assertion failures surface as
// endpoint.TypeAssertError.
func NewSSEServerTyped[Req any](
	stream func(ctx context.Context, request Req, s *SSEStream) error,
	dec func(*http.Request) (Req, error),
	options ...ServerOption,
) *SSEServer {
	if stream == nil || dec == nil {
		panic("essential parameters cannot be nil")
	}
	return NewSSEServer(
		func(ctx context.Context, request any, s *SSEStream) error {
			typed, ok := request.(Req)
			if !ok {
				return endpoint.NewTypeAssertError[Req](request)
			}
			return stream(ctx, typed, s)
		},
		func(_ context.Context, r *http.Request) (any, error) {
			return dec(r)
		},
		options...,
	)
}

// ServeHTTP implements http.Handler.
func (s *SSEServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	iw := &InterceptingWriter{ResponseWriter: w, code: http.StatusOK}
	responseWriter := iw.reimplementInterfaces()
	if len(s.cfg.finalizer) > 0 {
		defer func() {
			for _, f := range s.cfg.finalizer {
				f(ctx, r, iw)
			}
		}()
	}

	for _, f := range s.cfg.before {
		ctx = f(ctx, r)
	}

	request, err := s.cfg.dec(ctx, r)
	if err != nil {
		s.cfg.errorHandler.Handle(ctx, err)
		s.cfg.errorEncoder(ctx, err, responseWriter)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		streamErr := errors.New("sse: streaming unsupported")
		s.cfg.errorHandler.Handle(ctx, streamErr)
		s.cfg.errorEncoder(ctx, streamErr, responseWriter)
		return
	}

	// Stable: http.sse-headers — a stream answers 200 with text/event-stream, Cache-Control no-cache and X-Accel-Buffering no.
	// Covered by: TestSSEServer_WritesEventsAndHeaders, TestSSEServer_DecodeFailureBeforeHeaders
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// Disable proxy response buffering (e.g. nginx) so events reach the
	// client as they are written.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stream := &SSEStream{w: w, flusher: flusher}
	if err := s.stream(ctx, request, stream); err != nil {
		s.cfg.errorHandler.Handle(ctx, err)
	}
}
