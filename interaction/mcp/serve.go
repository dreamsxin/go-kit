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
// /mcp on the given address. If SessionTTL is configured on the handler,
// background cleanup is started automatically. The call blocks until the
// server exits.
//
//	rt := interaction.NewRuntime()
//	_ = rt.RegisterTool(/* ... */)
//	log.Fatal(mcp.ListenAndServe(":8080", rt))
func ListenAndServe(addr string, rt *interaction.Runtime) error {
	mux := http.NewServeMux()
	h := NewStreamableHandler(rt)
	mux.Handle("/mcp", h)
	h.StartCleanup()
	return http.ListenAndServe(addr, mux)
}

// ServeStreamable starts an HTTP server like ListenAndServe but returns the
// underlying *StreamableHandler so the caller can send server-initiated
// notifications or sampling requests during tool execution.
//
// Because http.ListenAndServe blocks, the handler is returned alongside the
// error (which is nil only after the server shuts down). Typical usage starts
// the server in a goroutine:
//
//	h, _ := mcp.ServeStreamable(":8080", rt)
//	// h is now available for h.StartCleanup(), notifications, etc.
func ServeStreamable(addr string, rt *interaction.Runtime) (*StreamableHandler, error) {
	h := NewStreamableHandler(rt)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return h, http.ListenAndServe(addr, mux)
}
