package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/dreamsxin/go-kit/interaction"
)

const (
	headerSessionID       = "Mcp-Session-Id"
	headerProtocolVersion = "MCP-Protocol-Version"
)

// StreamableHandler is a Streamable HTTP MCP transport that supports:
//   - POST for client JSON-RPC messages (requests, notifications, responses)
//   - GET  for persistent SSE streams (server-initiated messages like sampling)
//   - DELETE for explicit session termination
//
// It manages MCP sessions, SSE streams, and bidirectional communication.
type StreamableHandler struct {
	core    dispatchCore
	Sampler *Sampler
	store   *sessionStore

	// activePostStreams tracks the current POST-initiated SSE stream per session.
	activePostStreams sync.Map // sessionID -> *sseWriter
}

// NewStreamableHandler creates a StreamableHandler backed by the given runtime.
func NewStreamableHandler(runtime *interaction.Runtime) *StreamableHandler {
	if runtime == nil {
		runtime = interaction.NewRuntime(nil, nil, nil)
	}
	return &StreamableHandler{
		core:    dispatchCore{Runtime: runtime, logLevel: "info"},
		Sampler: NewSampler(),
		store:   newSessionStore(),
	}
}

func (h *StreamableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r)
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		w.Header().Set("Allow", "POST, GET, DELETE")
		writeHTTPError(w, http.StatusMethodNotAllowed, "method_not_allowed", "expected POST, GET, or DELETE")
	}
}

// ─── POST handler ────────────────────────────────────────────────────────────

func (h *StreamableHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Read raw body first so we can extract response fields that the
	// request struct does not model (result, error).
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}

	var req request
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}

	sessionID := r.Header.Get(headerSessionID)

	// ── initialize (no session yet) ──────────────────────────────────
	if req.Method == "initialize" {
		sess, err := h.store.create()
		if err != nil {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32603, "internal error", err.Error())})
			return
		}
		h.Sampler.RegisterSession(sess.ID)

		var initParams struct {
			Capabilities map[string]any `json:"capabilities"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &initParams)
		}
		sess.clientCaps = initParams.Capabilities

		w.Header().Set(headerSessionID, sess.ID)
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: h.core.buildInitializeResult()})
		return
	}

	// ── all other methods require a session ──────────────────────────
	if sessionID == "" {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", "Mcp-Session-Id header is required")})
		return
	}
	sess, ok := h.store.get(sessionID)
	if !ok {
		writeHTTPError(w, http.StatusNotFound, "session_not_found", "session not found; re-initialize")
		return
	}

	// ── JSON-RPC response (to server-initiated request like sampling) ─
	if req.Method == "" {
		h.deliverSamplingResponse(sess, rawBody)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// ── notifications (no id) ────────────────────────────────────────
	if req.ID == nil {
		h.handleNotification(sess, req)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// ── requests (have id) ───────────────────────────────────────────
	accept := r.Header.Get("Accept")
	if containsSSE(accept) {
		h.handleRequestSSE(w, r, sess, req)
	} else {
		resp := h.core.dispatch(r.Context(), req)
		writeResponse(w, resp)
	}
}

func (h *StreamableHandler) handleRequestSSE(w http.ResponseWriter, r *http.Request, sess *sseSession, req request) {
	sw, err := newSSEWriter(w)
	if err != nil {
		resp := h.core.dispatch(r.Context(), req)
		writeResponse(w, resp)
		return
	}

	h.activePostStreams.Store(sess.ID, sw)
	defer h.activePostStreams.Delete(sess.ID)

	resp := h.core.dispatch(r.Context(), req)
	respJSON, _ := json.Marshal(resp)
	_ = sw.writeEvent(respJSON)
}

// ─── GET handler ─────────────────────────────────────────────────────────────

func (h *StreamableHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(headerSessionID)
	if sessionID == "" {
		writeHTTPError(w, http.StatusBadRequest, "invalid_request", "Mcp-Session-Id header is required")
		return
	}
	sess, ok := h.store.get(sessionID)
	if !ok {
		writeHTTPError(w, http.StatusNotFound, "session_not_found", "session not found")
		return
	}

	sw, err := newSSEWriter(w)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writerID := "get-" + sessionID
	sess.addGETWriter(writerID, sw)
	defer sess.removeGETWriter(writerID)

	select {
	case <-r.Context().Done():
	case <-sw.Done():
	}
}

// ─── DELETE handler ──────────────────────────────────────────────────────────

func (h *StreamableHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get(headerSessionID)
	if sessionID == "" {
		writeHTTPError(w, http.StatusBadRequest, "invalid_request", "Mcp-Session-Id header is required")
		return
	}
	h.Sampler.UnregisterSession(sessionID)
	h.store.remove(sessionID)
	w.WriteHeader(http.StatusAccepted)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (h *StreamableHandler) handleNotification(sess *sseSession, req request) {
	switch req.Method {
	case "notifications/initialized":
		sess.mu.Lock()
		sess.initialized = true
		sess.mu.Unlock()
	}
}

func (h *StreamableHandler) deliverSamplingResponse(sess *sseSession, rawBody []byte) {
	var rr struct {
		ID     any                 `json:"id"`
		Result CreateMessageResult `json:"result,omitempty"`
	}
	_ = json.Unmarshal(rawBody, &rr)

	idStr, ok := rr.ID.(string)
	if !ok {
		return
	}
	h.Sampler.DeliverResponse(sess.ID, idStr, rr.Result)
}

// SendSamplingRequest sends a sampling/createMessage request to the connected
// MCP client on the given session. It blocks until the client responds or the
// context is cancelled.
//
// This is the primary API for tool implementations that need LLM completions
// from the client during tool execution.
func (h *StreamableHandler) SendSamplingRequest(ctx context.Context, sessionID string, req CreateMessageRequest) (CreateMessageResult, error) {
	sendFn := func(data json.RawMessage) error {
		if sw, ok := h.activePostStreams.Load(sessionID); ok {
			return sw.(*sseWriter).writeEvent(data)
		}
		sess, ok := h.store.get(sessionID)
		if !ok {
			return fmt.Errorf("mcp: session %q not found", sessionID)
		}
		return sess.broadcastToGET(data)
	}
	return h.Sampler.CreateMessage(ctx, sessionID, req, sendFn)
}

func containsSSE(accept string) bool {
	for _, v := range splitAccept(accept) {
		if v == "text/event-stream" {
			return true
		}
	}
	return false
}

func splitAccept(s string) []string {
	var out []string
	for _, part := range splitString(s, ',') {
		trimmed := trimString(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitString(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimString(s string) string {
	start := 0
	end := len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
