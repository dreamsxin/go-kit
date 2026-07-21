package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

const (
	headerSessionID       = "Mcp-Session-Id"
	headerProtocolVersion = "MCP-Protocol-Version"
	defaultMaxPostBody    = 4 << 20
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

	// SessionTTL configures how long a session may remain idle before being
	// automatically expired. When zero, sessions are never expired.
	SessionTTL time.Duration

	// cleanupInterval overrides the default cleanup tick interval.
	// When zero, defaults to SessionTTL/2 with a minimum of 30 seconds.
	cleanupInterval time.Duration

	// MaxPostBodyBytes caps Streamable HTTP POST payloads. When zero, a safe
	// default is used.
	MaxPostBodyBytes int64

	// AllowedOrigins permits browser clients from the listed origins. Requests
	// without Origin are allowed; same-origin requests are always allowed.
	AllowedOrigins []string

	cleanupMu     sync.Mutex
	cleanupCancel context.CancelFunc
	cleanupWG     sync.WaitGroup
}

// NewStreamableHandler creates a StreamableHandler backed by the given runtime.
func NewStreamableHandler(runtime *interaction.Runtime) *StreamableHandler {
	if runtime == nil {
		runtime = interaction.NewRuntime()
	}
	return &StreamableHandler{
		core:    dispatchCore{Runtime: runtime},
		Sampler: NewSampler(),
		store:   newSessionStore(),
	}
}

// ServeHTTP dispatches an HTTP request to the appropriate handler based on
// the HTTP method (POST, GET, DELETE).
func (h *StreamableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(headerProtocolVersion, protocolVersion)
	if err := h.validateOrigin(r); err != nil {
		writeHTTPError(w, http.StatusForbidden, "origin_not_allowed", err.Error())
		return
	}
	if err := validateProtocolVersion(r); err != nil {
		writeHTTPError(w, http.StatusBadRequest, "unsupported_protocol_version", err.Error())
		return
	}

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
	body := http.MaxBytesReader(w, r.Body, h.maxPostBodyBytes())
	defer body.Close()

	// Read raw body first so we can extract response fields that the
	// request struct does not model (result, error).
	rawBody, err := io.ReadAll(body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "request_too_large", fmt.Sprintf("request body exceeds %d bytes", h.maxPostBodyBytes()))
			return
		}
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}

	var req request
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}
	if req.JSONRPC != jsonRPCVersion {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", "jsonrpc must be 2.0")})
		return
	}

	sessionID := r.Header.Get(headerSessionID)

	// ── initialize (no session yet) ──────────────────────────────────
	if req.Method == "initialize" {
		if req.ID == nil {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32600, "invalid request", "initialize must include an id")})
			return
		}
		var initParams struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
		}
		if err := json.Unmarshal(req.Params, &initParams); err != nil {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32602, "invalid argument", "initialize params are required")})
			return
		}
		if initParams.ProtocolVersion != protocolVersion {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32602, "unsupported protocol version", fmt.Sprintf("server supports %q", protocolVersion))})
			return
		}
		sess, err := h.store.create()
		if err != nil {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32603, "internal error", err.Error())})
			return
		}
		runtimeSession, err := h.core.Runtime.StartSession(r.Context(), "mcp:"+sess.ID, nil)
		if err != nil {
			h.store.remove(sess.ID)
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32603, "internal error", err.Error())})
			return
		}
		h.Sampler.RegisterSession(sess.ID)

		sess.mu.Lock()
		sess.runtimeID = string(runtimeSession.ID)
		sess.clientCaps = initParams.Capabilities
		sess.protocol = initParams.ProtocolVersion
		sess.mu.Unlock()

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
	ctx := context.WithValue(r.Context(), mcpSessionContextKey{}, sess)

	// ── JSON-RPC response (to server-initiated request like sampling) ─
	if req.Method == "" {
		if !sess.isInitialized() {
			writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", "session is not initialized")})
			return
		}
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
	if !sess.isInitialized() {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", "notifications/initialized must be sent before requests")})
		return
	}

	// ── requests (have id) ───────────────────────────────────────────
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/event-stream") {
		h.handleRequestSSE(w, r.WithContext(ctx), sess, req)
	} else {
		resp := h.core.dispatch(ctx, req)
		writeResponse(w, resp)
	}
}

func (h *StreamableHandler) handleRequestSSE(w http.ResponseWriter, r *http.Request, sess *sseSession, req request) {
	writerID, err := newSSEWriterID("post", sess.ID)
	if err != nil {
		resp := h.core.dispatch(r.Context(), req)
		writeResponse(w, resp)
		return
	}

	sw, err := newSSEWriter(w)
	if err != nil {
		resp := h.core.dispatch(r.Context(), req)
		writeResponse(w, resp)
		return
	}

	sess.addPostWriter(writerID, sw)
	defer sess.removePostWriter(writerID)

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
	if !sess.isInitialized() {
		writeHTTPError(w, http.StatusConflict, "session_not_initialized", "notifications/initialized must be sent before opening an SSE stream")
		return
	}

	writerID, err := newSSEWriterID("get", sessionID)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	sw, err := newSSEWriter(w)
	if err != nil {
		writeHTTPError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

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
	h.releaseRuntimeSession(r.Context(), h.store.remove(sessionID))
	w.WriteHeader(http.StatusAccepted)
}

func (h *StreamableHandler) releaseRuntimeSession(ctx context.Context, sess *sseSession) {
	if sess == nil {
		return
	}
	runtimeID := sess.runtimeSessionID()
	if runtimeID == "" {
		return
	}
	_ = h.core.Runtime.ReleaseSession(ctx, interaction.SessionID(runtimeID))
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
		sess, ok := h.store.get(sessionID)
		if !ok {
			return fmt.Errorf("mcp: session %q not found", sessionID)
		}
		if !sess.isInitialized() {
			return fmt.Errorf("mcp: session %q is not initialized", sessionID)
		}
		if !sess.hasClientCapability("sampling") {
			return fmt.Errorf("mcp: client session %q did not declare sampling capability", sessionID)
		}
		if delivered, err := sess.writeToPOST(data); delivered || err != nil {
			return err
		}
		return sess.writeToGET(data)
	}
	return h.Sampler.CreateMessage(ctx, sessionID, req, sendFn)
}

func (h *StreamableHandler) validateOrigin(r *http.Request) error {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	origin = strings.TrimRight(origin, "/")
	for _, allowed := range h.AllowedOrigins {
		if origin == strings.TrimRight(strings.TrimSpace(allowed), "/") {
			return nil
		}
	}
	if origin == "http://"+r.Host || origin == "https://"+r.Host {
		return nil
	}
	return fmt.Errorf("origin %q is not allowed", origin)
}

func (h *StreamableHandler) maxPostBodyBytes() int64 {
	if h.MaxPostBodyBytes > 0 {
		return h.MaxPostBodyBytes
	}
	return defaultMaxPostBody
}

func validateProtocolVersion(r *http.Request) error {
	version := strings.TrimSpace(r.Header.Get(headerProtocolVersion))
	if version == "" || version == protocolVersion {
		return nil
	}
	return fmt.Errorf("unsupported MCP protocol version %q; server supports %q", version, protocolVersion)
}

// StartCleanup begins a background goroutine that periodically expires idle
// sessions. It checks every SessionTTL/2 (minimum 30 seconds). Call StopCleanup
// to terminate the goroutine. If SessionTTL is zero, this is a no-op.
func (h *StreamableHandler) StartCleanup() {
	if h.SessionTTL <= 0 {
		return
	}
	h.cleanupMu.Lock()
	defer h.cleanupMu.Unlock()
	if h.cleanupCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cleanupCancel = cancel

	interval := h.cleanupInterval
	if interval == 0 {
		interval = h.SessionTTL / 2
		if interval < 30*time.Second {
			interval = 30 * time.Second
		}
	}

	h.cleanupWG.Add(1)
	go func() {
		defer h.cleanupWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, id := range h.store.expiredIDs(h.SessionTTL) {
					h.Sampler.UnregisterSession(id)
					h.releaseRuntimeSession(context.Background(), h.store.remove(id))
				}
			}
		}
	}()
}

// StopCleanup terminates the background session cleanup goroutine.
func (h *StreamableHandler) StopCleanup() {
	h.cleanupMu.Lock()
	defer h.cleanupMu.Unlock()
	if h.cleanupCancel == nil {
		return
	}
	h.cleanupCancel()
	h.cleanupWG.Wait()
	h.cleanupCancel = nil
}
