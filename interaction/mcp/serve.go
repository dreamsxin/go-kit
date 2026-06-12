package mcp

import (
	"net/http"

	"github.com/dreamsxin/go-kit/interaction"
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
// /mcp on the given address. It blocks until the server exits.
//
//	rt := interaction.NewRuntime()
//	_ = rt.RegisterTool(/* ... */)
//	log.Fatal(mcp.ListenAndServe(":8080", rt))
func ListenAndServe(addr string, rt *interaction.Runtime) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", NewStreamableHandler(rt))
	return http.ListenAndServe(addr, mux)
}

// ServeStreamable starts an HTTP server like ListenAndServe but returns the
// underlying *StreamableHandler so the caller can send server-initiated
// notifications or sampling requests during tool execution. Note that
// http.ListenAndServe blocks, so the handler value is only useful if the
// listener is set up out-of-band (e.g. via http.Server in another goroutine).
func ServeStreamable(addr string, rt *interaction.Runtime) (*StreamableHandler, error) {
	h := NewStreamableHandler(rt)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return h, http.ListenAndServe(addr, mux)
}
