package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// SSEEvent is one decoded Server-Sent Event: the event name and the JSON
// payload decoded into the stream's event type.
type SSEEvent[T any] struct {
	Name string
	Data T
}

// HandleSSE connects to a Server-Sent Events stream and decodes it. It is the
// client-side mirror of the server's typed SSE routes: the wire carries
// `event:` and `data:` fields, the data is JSON decoded into T, and one call
// to onEvent delivers one event. The call blocks until the server ends the
// stream, the response fails, or ctx is cancelled — the same blocking shape
// the server's stream function has, so a caller reads symmetric code on both
// sides of the wire.
//
// The URL is sent as written; request headers flow through ClientBefore
// (Authorization, tracing). Non-2xx answers surface as *HTTPStatusError.
// Comment lines (": keepalive"), id, and retry fields are consumed per the
// SSE framing and never reach onEvent. An event whose data does not decode
// into T ends the stream with the decode error.
//
// Stable: http.sse-client — HandleSSE connects with GET and
// Accept: text/event-stream, delivers one SSEEvent per framed event with the
// JSON data decoded into T, consumes comments/id/retry fields, and returns
// *HTTPStatusError for non-2xx answers. A stream typed string receives the
// raw payload instead of a JSON decode.
// Covered by: TestHandleSSEDecodesEvents, TestHandleSSEEndsWhenServerCloses,
// TestHandleSSECancel, TestHandleSSENon200, TestHandleSSEClientBeforeHeaders,
// TestHandleSSEConsumesCommentsAndRetry
func HandleSSE[T any](ctx context.Context, rawURL string, onEvent func(context.Context, SSEEvent[T]) error, options ...ClientOption) error {
	c := NewExplicitClient(
		func(_ context.Context, r *http.Request, _ any) (*http.Request, error) { return r, nil },
		func(_ context.Context, _ *http.Response) (any, error) { return nil, nil },
		options...,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("sse client: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for _, f := range c.before {
		ctx = f(ctx, req)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newHTTPStatusError(resp)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		return fmt.Errorf("sse client: content type %q is not text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), int(DefaultMaxJSONResponseBytes))

	var (
		name      string
		dataLines []string
		hasData   bool
	)
	dispatch := func() error {
		defer func() {
			name = ""
			dataLines = nil
			hasData = false
		}()
		if !hasData {
			return nil // comment-only or fieldless frame: nothing to deliver
		}
		var data T
		payload := strings.Join(dataLines, "\n")
		if strings.TrimSpace(payload) == "" {
			return onEvent(ctx, SSEEvent[T]{Name: name})
		}
		// A stream whose event type is string receives the raw payload — the
		// server's Data writes raw text lines. Every other type JSON-decodes,
		// which is what the server's EventJSON writes.
		if reflect.TypeOf(data).Kind() == reflect.String {
			data = any(payload).(T)
		} else if err := json.Unmarshal([]byte(payload), &data); err != nil {
			return fmt.Errorf("sse client: decode event %q: %w", name, err)
		}
		return onEvent(ctx, SSEEvent[T]{Name: name, Data: data})
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"): // comment, keeps the connection warm
		case strings.HasPrefix(line, "data:"):
			hasData = true
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimPrefix(strings.TrimPrefix(line, "event:"), " ")
		default: // id, retry: consumed, never delivered
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return dispatch() // a stream may end without the closing blank line
}
