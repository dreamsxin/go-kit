package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/dreamsxin/go-kit/interaction"
)

// Handler is a simple POST-only HTTP handler that speaks MCP JSON-RPC
// to an interaction Runtime. For full Streamable HTTP with SSE and
// sampling support, use StreamableHandler instead.
type Handler struct {
	core dispatchCore
	mu   sync.RWMutex
	ready bool
}

// NewHandler creates a Handler backed by the given runtime.
func NewHandler(runtime *interaction.Runtime) *Handler {
	if runtime == nil {
		runtime = interaction.NewRuntime(nil, nil, nil)
	}
	return &Handler{
		core: dispatchCore{Runtime: runtime, logLevel: "info"},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeHTTPError(w, http.StatusMethodNotAllowed, "method_not_allowed", "MCP endpoint expects POST")
		return
	}
	defer r.Body.Close()

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}
	resp := h.handle(r.Context(), req)
	if req.ID == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeResponse(w, resp)
}

func (h *Handler) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		return response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: h.core.buildInitializeResult()}
	case "notifications/initialized":
		h.mu.Lock()
		h.ready = true
		h.mu.Unlock()
		return response{}
	}
	return h.core.dispatch(ctx, req)
}
