package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// SSEWriter writes Server-Sent Events to an HTTP response. The zero value is
// not usable; HandleSSE creates one and passes it to the stream function.
//
// Methods are not safe for concurrent use: event writes from multiple
// goroutines must be serialized by the caller.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// Data writes one unnamed data event. Multi-line data is written as one
// "data:" line per line, as required by the SSE format.
func (sw *SSEWriter) Data(data string) error {
	return sw.writeEvent("", data)
}

// Event writes one named event.
func (sw *SSEWriter) Event(name, data string) error {
	return sw.writeEvent(name, data)
}

// EventJSON marshals v as JSON and writes it as one named event.
func (sw *SSEWriter) EventJSON(name string, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return sw.writeEvent(name, string(payload))
}

// Comment writes a comment line. Comments are ignored by clients and are the
// standard keep-alive heartbeat for streams behind proxies that time out
// idle connections.
func (sw *SSEWriter) Comment(text string) error {
	_, err := fmt.Fprintf(sw.w, ": %s\n", text)
	if err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// Retry advises clients to wait the given milliseconds before reconnecting
// after a dropped connection.
func (sw *SSEWriter) Retry(milliseconds int) error {
	_, err := fmt.Fprintf(sw.w, "retry: %d\n", milliseconds)
	if err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

func (sw *SSEWriter) writeEvent(name, data string) error {
	var b strings.Builder
	if name != "" {
		b.WriteString("event: " + name + "\n")
	}
	for i, line := range strings.Split(data, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("data: " + line)
	}
	b.WriteString("\n\n")
	if _, err := sw.w.Write([]byte(b.String())); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// HandleSSE registers a Server-Sent Events stream at pattern. The stream
// function receives the request context, which is cancelled when the client
// disconnects, and an SSEWriter that flushes after every event.
//
// HandleSSE is a raw HTTP escape hatch like Service.Handle: endpoint
// middleware does not apply. The response has already started when the
// stream function runs, so errors it returns can only be logged; emit a
// terminal event when clients need to learn about a failure.
//
// Example:
//
//	kit.HandleSSE(svc, "GET /events", func(ctx context.Context, w *kit.SSEWriter) error {
//		ticker := time.NewTicker(time.Second)
//		defer ticker.Stop()
//		for i := 0; ; i++ {
//			select {
//			case <-ctx.Done():
//				return nil
//			case <-ticker.C:
//				if err := w.EventJSON("progress", map[string]int{"step": i}); err != nil {
//					return err
//				}
//			}
//		}
//	})
func HandleSSE(s *Service, pattern string, stream func(ctx context.Context, w *SSEWriter) error) {
	if stream == nil {
		panic("kit: SSE stream function cannot be nil")
	}
	s.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		// Disable proxy response buffering (e.g. nginx) so events reach the
		// client as they are written.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sse := &SSEWriter{w: w, flusher: flusher}
		if err := stream(r.Context(), sse); err != nil {
			log.Printf("kit: SSE stream %s ended with error: %v", r.URL.Path, err)
		}
	}))
}
