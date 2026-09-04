package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// NewHandler returns a *StreamableHandler — the canonical MCP handler that
// supports the full Streamable HTTP transport (POST/GET/DELETE, SSE,
// sessions, sampling, notifications, completions).
//
// This is an alias for NewStreamableHandler retained as the simplest entry
// point.
func NewHandler(rt *interaction.Runtime) *StreamableHandler {
	return NewStreamableHandler(rt)
}

// ListenAndServe starts an HTTP server with a StreamableHandler mounted at
// /mcp on the given address. If SessionTTL is configured on the handler,
// background cleanup is started automatically. The call blocks until the
// server exits.
//
//	rt := interaction.NewRuntime()
//	_ = rt.RegisterTool(/* ... */)
//	log.Fatal(mcp.ListenAndServe(":8080", rt))
func ListenAndServe(addr string, rt *interaction.Runtime) error {
	srv, h := NewHTTPServer(addr, rt)
	h.StartCleanup()
	defer h.Close()
	return srv.ListenAndServe()
}

// NewHTTPServer constructs an HTTP server and its MCP handler without
// starting either one. The handler is returned so callers can send
// server-initiated notifications while the server is running.
func NewHTTPServer(addr string, rt *interaction.Runtime) (*http.Server, *StreamableHandler) {
	h := NewStreamableHandler(rt)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return &http.Server{Addr: addr, Handler: mux}, h
}

// Serve starts an MCP HTTP server and shuts it down when ctx is cancelled.
// It owns the StreamableHandler created for the duration of the call and
// releases all sessions before returning. Cancellation is a normal shutdown
// and returns nil unless the server or graceful shutdown fails.
func Serve(ctx context.Context, addr string, rt *interaction.Runtime) error {
	if ctx == nil {
		return fmt.Errorf("mcp: nil serve context")
	}
	srv, h := NewHTTPServer(addr, rt)
	h.StartCleanup()
	defer h.Close()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(lis) }()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		serveErr := <-serveErr
		if shutdownErr != nil {
			return shutdownErr
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}

// ServeStreamable starts an HTTP server like ListenAndServe but returns the
// underlying *StreamableHandler so the caller can send server-initiated
// notifications or sampling requests during tool execution.
//
// Deprecated: Because http.ListenAndServe blocks, the returned handler is
// only available after the server shuts down, making it unusable for
// notifications during tool execution. Use NewStreamableHandler directly
// instead:
//
//	h := mcp.NewStreamableHandler(rt)
//	mux := http.NewServeMux()
//	mux.Handle("/mcp", h)
//	h.StartCleanup()
//	http.ListenAndServe(":8080", mux)
func ServeStreamable(addr string, rt *interaction.Runtime) (*StreamableHandler, error) {
	srv, h := NewHTTPServer(addr, rt)
	h.StartCleanup()
	defer h.Close()
	return h, srv.ListenAndServe()
}
