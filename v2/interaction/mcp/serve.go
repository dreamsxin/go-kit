package mcp

import (
	"net/http"

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
	h := NewStreamableHandler(rt)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	return h, http.ListenAndServe(addr, mux)
}
