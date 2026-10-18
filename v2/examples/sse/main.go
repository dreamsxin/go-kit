// Package main demonstrates a Server-Sent Events stream on the typed kit
// path: one handler, one room, the client's connection held open while the
// service pushes events.
//
// Concepts shown:
//   - kit.HandleSSETyped registers a stream whose decode reads the request
//     directly (path and query values), so path parameters work on streams
//   - the stream runs inside the endpoint chain, so component middleware and
//     recorders observe the whole stream as one request
//   - kit.Stopping(ctx) ends the stream during graceful shutdown instead of
//     leaving it to the drain deadline
//
// Run:
//
//	go run ./examples/sse
//
// Watch the stream (in a second terminal):
//
//	curl -N http://localhost:8080/rooms/lobby/events
//
// Every two seconds the room pushes a tick; Ctrl-C shuts the service down
// and the stream ends with the process.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/kit"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"
)

// tickEvent is one pushed message. The SSE encoder writes it as JSON.
type tickEvent struct {
	Room string    `json:"room"`
	Seq  int       `json:"seq"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

type streamRequest struct {
	Room string `form:"room" json:"room"`
}

// decodeStreamRequest reads the path value directly: the SSE decoder receives
// the *http.Request, so path and query parameters are read here.
func decodeStreamRequest(r *http.Request) (streamRequest, error) {
	return streamRequest{Room: r.PathValue("room")}, nil
}

func run(ctx context.Context, addr string) error {
	svc, err := kit.NewHTTP(addr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	if err != nil {
		return err
	}

	ticks := make(chan tickEvent)
	go func() {
		defer close(ticks)
		var seq int
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case at := <-ticker.C:
				seq++
				ticks <- tickEvent{Room: "*", Seq: seq, At: at, Text: "tick"}
			}
		}
	}()

	// The stream reads from the shared channel; the room from the path is
	// echoed back so the client can tell rooms apart.
	kit.HandleSSETyped(svc, "GET /rooms/{room}/events", func(
		ctx context.Context, req streamRequest, w *httpserver.SSEStream,
	) error {
		for {
			select {
			case <-kit.Stopping(ctx):
				return w.Comment("server shutting down")
			case evt, ok := <-ticks:
				if !ok {
					return nil
				}
				evt.Room = req.Room
				if err := w.EventJSON("tick", evt); err != nil {
					return err
				}
			}
		}
	}, decodeStreamRequest)

	// The plain health routes round the service out.
	kit.HandleJSON(svc, "GET /whoami", func(_ context.Context, _ struct{}) (string, error) {
		return "sse example", nil
	})

	log.Println("sse example listening on", addr)
	host, err := kit.NewHost(kit.WithLifecycle(svc))
	if err != nil {
		return err
	}
	return host.Run(ctx)
}

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *httpAddr); err != nil {
		log.Fatal(err)
	}
	_ = fmt.Sprint()
}
