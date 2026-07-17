package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/interaction"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func streamPost(t *testing.T, handler http.Handler, sessionID, method string, params any, acceptSSE bool) (int, http.Header, *bufio.Reader) {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	if sessionID != "" {
		req.Header.Set(headerSessionID, sessionID)
	}
	if acceptSSE {
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Header(), bufio.NewReader(rec.Body)
}

func streamPostJSON(t *testing.T, handler http.Handler, sessionID, method string, params any) (int, map[string]any) {
	t.Helper()
	code, _, reader := streamPost(t, handler, sessionID, method, params, false)
	var resp map[string]any
	_ = json.NewDecoder(reader).Decode(&resp)
	return code, resp
}

func initSession(t *testing.T, handler http.Handler) string {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("initialize: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	sid := rec.Header().Get(headerSessionID)
	if sid == "" {
		t.Fatal("initialize: no Mcp-Session-Id header")
	}
	return sid
}

// ─── StreamableHandler: initialization & sessions ────────────────────────────

func TestStreamableInitialize(t *testing.T) {
	h := NewStreamableHandler(nil)
	sid := initSession(t, h)
	if len(sid) < 10 {
		t.Fatalf("session ID too short: %s", sid)
	}
}

func TestStreamableRejectsUnsupportedProtocolHeader(t *testing.T) {
	h := NewStreamableHandler(nil)
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerProtocolVersion, "2024-11-05")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unsupported_protocol_version") {
		t.Fatalf("body = %s, want unsupported_protocol_version", rec.Body.String())
	}
}

func TestStreamableAcceptsSupportedProtocolHeader(t *testing.T) {
	h := NewStreamableHandler(nil)
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerProtocolVersion, protocolVersion)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get(headerProtocolVersion); got != protocolVersion {
		t.Fatalf("%s = %q, want %q", headerProtocolVersion, got, protocolVersion)
	}
}

func TestStreamableRejectsOversizedPostBody(t *testing.T) {
	h := NewStreamableHandler(nil)
	h.MaxPostBodyBytes = 8
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "request_too_large") {
		t.Fatalf("body = %s, want request_too_large", rec.Body.String())
	}
}

func TestStreamableGETAllowsMultipleConcurrentStreams(t *testing.T) {
	h := NewStreamableHandler(nil)
	sid := initSession(t, h)

	cancel1, done1 := startGETStream(t, h, sid)
	cancel2, done2 := startGETStream(t, h, sid)
	defer cancel1()
	defer cancel2()

	waitFor(t, func() bool {
		sess, ok := h.store.get(sid)
		if !ok {
			return false
		}
		sess.mu.RLock()
		defer sess.mu.RUnlock()
		return len(sess.getWriters) == 2
	})

	cancel1()
	cancel2()
	<-done1
	<-done2
}

func TestStreamableRequiresSession(t *testing.T) {
	h := NewStreamableHandler(nil)
	code, resp := streamPostJSON(t, h, "", "tools/list", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if resp["error"] == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestStreamableInvalidSession(t *testing.T) {
	h := NewStreamableHandler(nil)
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, "bogus-session")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStreamableToolsListWithSession(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "ping_tool",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: "pong"}, nil
		},
	})
	h := NewStreamableHandler(rt)
	sid := initSession(t, h)

	code, resp := streamPostJSON(t, h, sid, "tools/list", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}
}

// ─── StreamableHandler: notifications ────────────────────────────────────────

func TestStreamableNotification(t *testing.T) {
	h := NewStreamableHandler(nil)
	sid := initSession(t, h)

	body := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
}

// ─── StreamableHandler: DELETE ───────────────────────────────────────────────

func TestStreamableDelete(t *testing.T) {
	h := NewStreamableHandler(nil)
	sid := initSession(t, h)

	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}

	// Session should be gone now.
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}
	payload, _ := json.Marshal(body)
	req2 := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req2.Header.Set(headerSessionID, sid)
	req2.Header.Set("Accept", "application/json")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after delete", rec2.Code)
	}
}

// ─── StreamableHandler: SSE response ────────────────────────────────────────

func TestStreamableSSEResponse(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "echo",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: call.Input}, nil
		},
	})
	h := NewStreamableHandler(rt)
	sid := initSession(t, h)

	code, headers, reader := streamPost(t, h, sid, "tools/list", nil, true)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	ct := headers.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("content-type = %s, want text/event-stream", ct)
	}

	// Read the SSE event.
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read SSE line: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected SSE data line, got: %s", line)
	}
	data := strings.TrimPrefix(line, "data: ")
	var resp map[string]any
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("unmarshal SSE data: %v", err)
	}
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}
}

// ─── StreamableHandler: method not allowed ───────────────────────────────────

func TestStreamableMethodNotAllowed(t *testing.T) {
	h := NewStreamableHandler(nil)
	req := httptest.NewRequest(http.MethodPut, "/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// ─── Sampling ────────────────────────────────────────────────────────────────

func TestSamplerCreateAndDeliver(t *testing.T) {
	sampler := NewSampler()
	sampler.RegisterSession("sess-1")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Deliver the response asynchronously.
	go func() {
		time.Sleep(50 * time.Millisecond)
		ok := sampler.DeliverResponse("sess-1", "sampling-1", CreateMessageResult{
			Role:       "assistant",
			Content:    SamplingContent{Type: "text", Text: "Paris"},
			Model:      "test-model",
			StopReason: "endTurn",
		})
		if !ok {
			t.Error("DeliverResponse returned false")
		}
	}()

	result, err := sampler.CreateMessage(ctx, "sess-1", CreateMessageRequest{
		Messages:  []SamplingMessage{{Role: "user", Content: SamplingContent{Type: "text", Text: "Capital of France?"}}},
		MaxTokens: 100,
	}, func(data json.RawMessage) error {
		// Verify the request was sent.
		var msg map[string]any
		_ = json.Unmarshal(data, &msg)
		if msg["method"] != "sampling/createMessage" {
			t.Errorf("expected sampling/createMessage, got %v", msg["method"])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if result.Content.Text != "Paris" {
		t.Fatalf("result text = %q, want Paris", result.Content.Text)
	}
	if result.Model != "test-model" {
		t.Fatalf("result model = %q, want test-model", result.Model)
	}
}

func TestSamplerContextCancellation(t *testing.T) {
	sampler := NewSampler()
	sampler.RegisterSession("sess-2")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := sampler.CreateMessage(ctx, "sess-2", CreateMessageRequest{
		Messages:  []SamplingMessage{{Role: "user", Content: SamplingContent{Type: "text", Text: "test"}}},
		MaxTokens: 10,
	}, func(data json.RawMessage) error { return nil })
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestSamplerUnregisteredSession(t *testing.T) {
	sampler := NewSampler()
	ctx := context.Background()
	_, err := sampler.CreateMessage(ctx, "nonexistent", CreateMessageRequest{
		Messages:  []SamplingMessage{{Role: "user", Content: SamplingContent{Type: "text", Text: "test"}}},
		MaxTokens: 10,
	}, func(data json.RawMessage) error { return nil })
	if err == nil {
		t.Fatal("expected error for unregistered session")
	}
}

func TestSamplerUnregisterClosesPending(t *testing.T) {
	sampler := NewSampler()
	sampler.RegisterSession("sess-3")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := sampler.CreateMessage(ctx, "sess-3", CreateMessageRequest{
			Messages:  []SamplingMessage{{Role: "user", Content: SamplingContent{Type: "text", Text: "test"}}},
			MaxTokens: 10,
		}, func(data json.RawMessage) error { return nil })
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)
	sampler.UnregisterSession("sess-3")

	err := <-errCh
	if err == nil {
		t.Fatal("expected error after session unregister")
	}
}

// ─── Sampling integration with StreamableHandler ─────────────────────────────

func TestStreamableSamplingResponseDelivery(t *testing.T) {
	h := NewStreamableHandler(nil)
	sid := initSession(t, h)

	// Simulate: server sends sampling request, client responds via POST.
	// First, register a pending request manually.
	h.Sampler.RegisterSession(sid + "-extra")
	// The session was already registered by initSession, so let's use the real one.

	// Deliver a response as if the client sent it.
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      "sampling-99",
		"result": map[string]any{
			"role":       "assistant",
			"content":    map[string]any{"type": "text", "text": "hello"},
			"model":      "test",
			"stopReason": "endTurn",
		},
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// Should be accepted (even though no pending request matches).
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
}

// ─── SessionTTL ──────────────────────────────────────────────────────────────

func TestSessionTTL_ExpiredIDs(t *testing.T) {
	h := NewStreamableHandler(nil)
	h.SessionTTL = 50 * time.Millisecond
	sid := initSessionHelper(t, h)

	// Session should not be expired immediately.
	ids := h.store.expiredIDs(h.SessionTTL)
	if len(ids) != 0 {
		t.Fatalf("expected no expired sessions, got %d", len(ids))
	}

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	ids = h.store.expiredIDs(h.SessionTTL)
	if len(ids) != 1 || ids[0] != sid {
		t.Fatalf("expected expired session %q, got %v", sid, ids)
	}

	// Verify the session can still be accessed before cleanup runs.
	_, ok := h.store.get(sid)
	if !ok {
		t.Fatal("session should still exist before cleanup")
	}
}

func TestSessionTTL_StartStopCleanup(t *testing.T) {
	h := NewStreamableHandler(nil)
	h.SessionTTL = 50 * time.Millisecond
	h.cleanupInterval = 30 * time.Millisecond
	sid := initSessionHelper(t, h)

	h.StartCleanup()
	defer h.StopCleanup()

	// Wait for the session to expire and cleanup to run.
	time.Sleep(200 * time.Millisecond)

	_, ok := h.store.get(sid)
	if ok {
		t.Fatal("session should have been cleaned up")
	}
}

func TestSessionTTL_ZeroDisablesExpiry(t *testing.T) {
	h := NewStreamableHandler(nil)
	// SessionTTL is zero by default.
	sid := initSessionHelper(t, h)

	// StartCleanup should be a no-op when SessionTTL is zero.
	h.StartCleanup()
	defer h.StopCleanup()

	time.Sleep(100 * time.Millisecond)

	_, ok := h.store.get(sid)
	if !ok {
		t.Fatal("session should still exist when SessionTTL is zero")
	}
}

func startGETStream(t *testing.T, h http.Handler, sessionID string) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	req.Header.Set(headerSessionID, sessionID)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()
	return cancel, done
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
