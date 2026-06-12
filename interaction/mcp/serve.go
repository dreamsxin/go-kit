package mcp

import (
	"net/http"

	"github.com/dreamsxin/go-kit/interaction"
)

// ListenAndServe starts an HTTP server with a simple POST-only MCP handler
// at /mcp. This is a convenience function for the minimal MCP server setup.
// For full Streamable HTTP transport (SSE, sampling, notifications), use
// ServeStreamable instead.
func ListenAndServe(addr string, rt *interaction.Runtime) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", NewHandler(rt))
	return http.ListenAndServe(addr, mux)
}

// ServeStreamable starts an HTTP server with a full StreamableHandler at /mcp,
// supporting POST/GET/DELETE with SSE streams, sessions, sampling, and
// server-initiated notifications. It returns the handler so callers can send
// notifications or sampling requests during tool execution.
func ServeStreamable(addr string, rt *interaction.Runtime) (*StreamableHandler, error) {
	h := NewStreamableHandler(rt)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	err := http.ListenAndServe(addr, mux)
	return h, err
}
