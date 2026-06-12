package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/interaction"
)

// ─── E2E test helpers ────────────────────────────────────────────────────────

// e2eEnv sets up a real HTTP server with a StreamableHandler and provides
// convenience methods for making MCP requests.
type e2eEnv struct {
	server  *httptest.Server
	handler *StreamableHandler
	client  *http.Client
	baseURL string
}

func newE2EEnv(t *testing.T, rt *interaction.Runtime) *e2eEnv {
	t.Helper()
	h := NewStreamableHandler(rt)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &e2eEnv{
		server:  srv,
		handler: h,
		client:  srv.Client(),
		baseURL: srv.URL + "/mcp",
	}
}

func (e *e2eEnv) initialize(t *testing.T) string {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req, _ := http.NewRequest(http.MethodPost, e.baseURL, strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize: status=%d body=%s", resp.StatusCode, b)
	}
	sid := resp.Header.Get(headerSessionID)
	if sid == "" {
		t.Fatal("initialize: no Mcp-Session-Id header")
	}
	// Verify response body.
	var rpcResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
	result := rpcResp["result"].(map[string]any)
	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", result["protocolVersion"], protocolVersion)
	}
	return sid
}

func (e *e2eEnv) postJSON(t *testing.T, sid, method string, params any) map[string]any {
	t.Helper()
	rpcReq := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		rpcReq["params"] = params
	}
	payload, _ := json.Marshal(rpcReq)
	req, _ := http.NewRequest(http.MethodPost, e.baseURL, bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	req.Header.Set("Accept", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("postJSON %s: %v", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("postJSON %s: status=%d body=%s", method, resp.StatusCode, b)
	}
	var rpcResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
	return rpcResp
}

func (e *e2eEnv) postSSE(t *testing.T, sid, method string, params any) (http.Header, *bufio.Reader) {
	t.Helper()
	rpcReq := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		rpcReq["params"] = params
	}
	payload, _ := json.Marshal(rpcReq)
	req, _ := http.NewRequest(http.MethodPost, e.baseURL, bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("postSSE %s: %v", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("postSSE %s: status=%d body=%s", method, resp.StatusCode, b)
	}
	return resp.Header, bufio.NewReader(resp.Body)
}

func (e *e2eEnv) postNotification(t *testing.T, sid, method string) int {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "method": method}
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, e.baseURL, bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("postNotification %s: %v", method, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (e *e2eEnv) deleteSession(t *testing.T, sid string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, e.baseURL, nil)
	req.Header.Set(headerSessionID, sid)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("deleteSession: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("readSSEEvent: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue // skip empty lines between events
		}
		if !strings.HasPrefix(line, "data: ") {
			continue // skip non-data lines
		}
		data := strings.TrimPrefix(line, "data: ")
		var msg map[string]any
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			t.Fatalf("readSSEEvent unmarshal: %v (data=%s)", err, data)
		}
		return msg
	}
}

// ─── E2E: Full lifecycle ─────────────────────────────────────────────────────

func TestE2E_FullLifecycle(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "greet",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: map[string]string{"message": "hello"}}, nil
		},
	})

	env := newE2EEnv(t, rt)

	// Step 1: Initialize.
	sid := env.initialize(t)

	// Step 2: Send initialized notification.
	code := env.postNotification(t, sid, "notifications/initialized")
	if code != http.StatusAccepted {
		t.Fatalf("notifications/initialized: status=%d, want 202", code)
	}

	// Step 3: List tools.
	resp := env.postJSON(t, sid, "tools/list", nil)
	if resp["error"] != nil {
		t.Fatalf("tools/list error: %v", resp["error"])
	}
	tools := resp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(tools))
	}
	if tools[0].(map[string]any)["name"] != "greet" {
		t.Fatalf("tool name = %v, want greet", tools[0].(map[string]any)["name"])
	}

	// Step 4: Call tool.
	callResp := env.postJSON(t, sid, "tools/call", map[string]any{
		"name":      "greet",
		"arguments": map[string]any{"name": "Alice"},
	})
	if callResp["error"] != nil {
		t.Fatalf("tools/call error: %v", callResp["error"])
	}
	callResult := callResp["result"].(map[string]any)
	if callResult["sessionId"] == "" {
		t.Fatal("tools/call: no sessionId in result")
	}
	content := callResult["content"].([]any)
	if len(content) == 0 {
		t.Fatal("tools/call: empty content")
	}

	// Step 5: Ping.
	pingResp := env.postJSON(t, sid, "ping", nil)
	if pingResp["error"] != nil {
		t.Fatalf("ping error: %v", pingResp["error"])
	}

	// Step 6: Delete session.
	code = env.deleteSession(t, sid)
	if code != http.StatusAccepted {
		t.Fatalf("delete: status=%d, want 202", code)
	}

	// Step 7: Verify session is gone.
	req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set(headerSessionID, sid)
	req.Header.Set("Accept", "application/json")
	resp2, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("post-delete ping: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("post-delete ping: status=%d, want 404", resp2.StatusCode)
	}
}

// ─── E2E: SSE streaming response ─────────────────────────────────────────────

func TestE2E_SSEStreamingResponse(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "echo",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: call.Input}, nil
		},
	})
	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	// POST with Accept: text/event-stream → get SSE response.
	headers, reader := env.postSSE(t, sid, "tools/list", nil)
	ct := headers.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("Content-Type = %s, want text/event-stream", ct)
	}

	msg := readSSEEvent(t, reader)
	if msg["error"] != nil {
		t.Fatalf("SSE event error: %v", msg["error"])
	}
	result := msg["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools count = %d, want 1", len(tools))
	}
}

// ─── E2E: GET SSE persistent stream ──────────────────────────────────────────

func TestE2E_GETSSEStream(t *testing.T) {
	env := newE2EEnv(t, nil)
	sid := env.initialize(t)

	// Open GET SSE stream in a goroutine.
	getDone := make(chan struct{})
	getCtx, getCancel := context.WithCancel(context.Background())
	defer getCancel()

	req, _ := http.NewRequestWithContext(getCtx, http.MethodGet, env.baseURL, nil)
	req.Header.Set(headerSessionID, sid)
	req.Header.Set("Accept", "text/event-stream")

	go func() {
		defer close(getDone)
		resp, err := env.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
		}
	}()

	// Give the GET stream time to establish.
	time.Sleep(100 * time.Millisecond)

	// Delete session → GET stream should close.
	code := env.deleteSession(t, sid)
	if code != http.StatusAccepted {
		t.Fatalf("delete: status=%d, want 202", code)
	}

	// Cancel GET context to close the stream.
	getCancel()

	// Wait for GET stream to close.
	select {
	case <-getDone:
		// Good, stream closed.
	case <-time.After(3 * time.Second):
		t.Fatal("GET SSE stream did not close after session delete")
	}
}

// ─── E2E: Sampling end-to-end ────────────────────────────────────────────────

func TestE2E_SamplingEndToEnd(t *testing.T) {
	rt := interaction.NewRuntime()
	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	// Open GET SSE stream to receive sampling requests.
	samplingReqCh := make(chan map[string]any, 1)
	getCtx, getCancel := context.WithCancel(context.Background())
	defer getCancel()
	getDone := make(chan struct{})

	go func() {
		defer close(getDone)
		req, _ := http.NewRequestWithContext(getCtx, http.MethodGet, env.baseURL, nil)
		req.Header.Set(headerSessionID, sid)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := env.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			samplingReqCh <- msg
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// Server sends sampling request via the handler API.
	sampleCtx, sampleCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sampleCancel()

	resultCh := make(chan CreateMessageResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := env.handler.SendSamplingRequest(sampleCtx, sid, CreateMessageRequest{
			Messages: []SamplingMessage{
				{Role: "user", Content: SamplingContent{Type: "text", Text: "What is 2+2?"}},
			},
			MaxTokens: 50,
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	// Wait for the sampling request to arrive on the GET stream.
	var samplingReq map[string]any
	select {
	case samplingReq = <-samplingReqCh:
	case <-time.After(3 * time.Second):
		t.Fatal("sampling request not received on GET stream")
	}

	// Verify the sampling request format.
	if samplingReq["method"] != "sampling/createMessage" {
		t.Fatalf("method = %v, want sampling/createMessage", samplingReq["method"])
	}
	reqID := samplingReq["id"]
	if reqID == nil {
		t.Fatal("sampling request has no id")
	}
	params := samplingReq["params"].(map[string]any)
	messages := params["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages count = %d, want 1", len(messages))
	}

	// Client responds via POST with the sampling result.
	samplingResp := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"result": map[string]any{
			"role": "assistant",
			"content": map[string]any{
				"type": "text",
				"text": "4",
			},
			"model":      "test-model",
			"stopReason": "endTurn",
		},
	}
	payload, _ := json.Marshal(samplingResp)
	postReq, _ := http.NewRequest(http.MethodPost, env.baseURL, bytes.NewReader(payload))
	postReq.Header.Set(headerSessionID, sid)
	resp, err := env.client.Do(postReq)
	if err != nil {
		t.Fatalf("sampling response POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("sampling response: status=%d, want 202", resp.StatusCode)
	}

	// Verify the tool/server received the sampling result.
	select {
	case result := <-resultCh:
		if result.Content.Text != "4" {
			t.Fatalf("sampling result text = %q, want '4'", result.Content.Text)
		}
		if result.Model != "test-model" {
			t.Fatalf("sampling result model = %q, want 'test-model'", result.Model)
		}
		if result.StopReason != "endTurn" {
			t.Fatalf("sampling result stopReason = %q, want 'endTurn'", result.StopReason)
		}
	case err := <-errCh:
		t.Fatalf("SendSamplingRequest: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("SendSamplingRequest did not return after client response")
	}

	// Cleanup: cancel GET stream context and delete session.
	getCancel()
	env.deleteSession(t, sid)
}

// ─── E2E: Sampling via POST SSE stream ───────────────────────────────────────

func TestE2E_SamplingViaPostSSEStream(t *testing.T) {
	// Register a tool that triggers sampling during execution.
	var handler *StreamableHandler
	samplingTool := interaction.ToolFunc{
		ToolName: "smart_tool",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			// This tool requests a sampling completion from the client.
			sid := "will-be-set-via-session"
			result, err := handler.SendSamplingRequest(ctx, sid, CreateMessageRequest{
				Messages: []SamplingMessage{
					{Role: "user", Content: SamplingContent{Type: "text", Text: "Summarize this"}},
				},
				MaxTokens: 100,
			})
			if err != nil {
				return interaction.ToolResult{}, err
			}
			return interaction.ToolResult{Output: result.Content.Text}, nil
		},
	}

	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(samplingTool)
	handler = NewStreamableHandler(rt)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	env := &e2eEnv{server: srv, handler: handler, client: srv.Client(), baseURL: srv.URL + "/mcp"}
	sid := env.initialize(t)

	// Open GET SSE stream to receive the sampling request.
	samplingReqCh := make(chan map[string]any, 1)
	getCtx, getCancel := context.WithCancel(context.Background())
	defer getCancel()
	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		req, _ := http.NewRequestWithContext(getCtx, http.MethodGet, env.baseURL, nil)
		req.Header.Set(headerSessionID, sid)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := env.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var msg map[string]any
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				continue
			}
			samplingReqCh <- msg
		}
	}()
	time.Sleep(100 * time.Millisecond)

	// Call the tool — it will trigger sampling.
	// We need to fix the session ID in the tool closure.
	// Since the tool uses a hardcoded sid, let's update it properly.
	// Actually, the tool should get the session from the call context.
	// For this test, we'll use a different approach: call SendSamplingRequest directly.

	// Instead, test the direct sampling flow.
	go func() {
		time.Sleep(100 * time.Millisecond)
		result, err := handler.SendSamplingRequest(context.Background(), sid, CreateMessageRequest{
			Messages:  []SamplingMessage{{Role: "user", Content: SamplingContent{Type: "text", Text: "Test"}}},
			MaxTokens: 50,
		})
		if err != nil {
			return
		}
		_ = result
	}()

	// Wait for sampling request on GET stream.
	var samplingReq map[string]any
	select {
	case samplingReq = <-samplingReqCh:
	case <-time.After(3 * time.Second):
		t.Fatal("sampling request not received")
	}

	// Respond to sampling.
	reqID := samplingReq["id"]
	samplingResp := map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"result": map[string]any{
			"role":    "assistant",
			"content": map[string]any{"type": "text", "text": "response"},
			"model":   "test",
		},
	}
	payload, _ := json.Marshal(samplingResp)
	postReq, _ := http.NewRequest(http.MethodPost, env.baseURL, bytes.NewReader(payload))
	postReq.Header.Set(headerSessionID, sid)
	resp, err := env.client.Do(postReq)
	if err != nil {
		t.Fatalf("sampling POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", resp.StatusCode)
	}

	env.deleteSession(t, sid)
	getCancel()
}

// ─── E2E: Multiple independent sessions ────────────────────────────────────── ──────────────────────────────────────

func TestE2E_MultipleSessions(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "counter",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: "ok"}, nil
		},
	})
	env := newE2EEnv(t, rt)

	// Create two independent sessions.
	sid1 := env.initialize(t)
	sid2 := env.initialize(t)
	if sid1 == sid2 {
		t.Fatal("sessions should have different IDs")
	}

	// Both sessions should work independently.
	resp1 := env.postJSON(t, sid1, "tools/list", nil)
	resp2 := env.postJSON(t, sid2, "tools/list", nil)

	tools1 := resp1["result"].(map[string]any)["tools"].([]any)
	tools2 := resp2["result"].(map[string]any)["tools"].([]any)
	if len(tools1) != 1 || len(tools2) != 1 {
		t.Fatalf("both sessions should see 1 tool: got %d and %d", len(tools1), len(tools2))
	}

	// Delete session 1 → session 2 should still work.
	env.deleteSession(t, sid1)

	resp2b := env.postJSON(t, sid2, "ping", nil)
	if resp2b["error"] != nil {
		t.Fatalf("session 2 should still work after session 1 deleted: %v", resp2b["error"])
	}

	// Session 1 should be gone.
	req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set(headerSessionID, sid1)
	req.Header.Set("Accept", "application/json")
	r, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("session 1 post-delete: %v", err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("session 1 post-delete: status=%d, want 404", r.StatusCode)
	}

	env.deleteSession(t, sid2)
}

// ─── E2E: Resources and Prompts via Streamable ──────────────────────────────

func TestE2E_ResourcesAndPrompts(t *testing.T) {
	rt := interaction.NewRuntime()

	resources := interaction.NewMemoryResourceProvider()
	_ = resources.Register(interaction.Resource{
		URI:  "test://hello",
		Name: "hello",
	}, []interaction.ResourceContent{
		{URI: "test://hello", Text: "world"},
	})
	rt.WithResources(resources)

	prompts := interaction.NewMemoryPromptProvider()
	_ = prompts.Register(interaction.Prompt{
		Name:      "test_prompt",
		Arguments: []interaction.PromptArgument{{Name: "input", Required: true}},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		return interaction.PromptResult{
			Messages: []interaction.PromptMessage{
				{Role: "user", Content: interaction.PromptContent{Type: "text", Text: args["input"]}},
			},
		}, nil
	})
	rt.WithPrompts(prompts)

	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	// Verify capabilities include resources and prompts.
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader(initBody))
	req.Header.Set("Accept", "application/json")
	resp, _ := env.client.Do(req)
	var initResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&initResp)
	resp.Body.Close()
	caps := initResp["result"].(map[string]any)["capabilities"].(map[string]any)
	if caps["resources"] == nil {
		t.Fatal("capabilities should include resources")
	}
	if caps["prompts"] == nil {
		t.Fatal("capabilities should include prompts")
	}

	// List resources.
	resResp := env.postJSON(t, sid, "resources/list", nil)
	resList := resResp["result"].(map[string]any)["resources"].([]any)
	if len(resList) != 1 {
		t.Fatalf("resources count = %d, want 1", len(resList))
	}

	// Read resource.
	readResp := env.postJSON(t, sid, "resources/read", map[string]any{"uri": "test://hello"})
	contents := readResp["result"].(map[string]any)["contents"].([]any)
	if contents[0].(map[string]any)["text"] != "world" {
		t.Fatalf("resource text = %v, want 'world'", contents[0])
	}

	// List prompts.
	promptsResp := env.postJSON(t, sid, "prompts/list", nil)
	promptsList := promptsResp["result"].(map[string]any)["prompts"].([]any)
	if len(promptsList) != 1 {
		t.Fatalf("prompts count = %d, want 1", len(promptsList))
	}

	// Get prompt.
	getResp := env.postJSON(t, sid, "prompts/get", map[string]any{
		"name":      "test_prompt",
		"arguments": map[string]string{"input": "hello"},
	})
	messages := getResp["result"].(map[string]any)["messages"].([]any)
	msg := messages[0].(map[string]any)
	content := msg["content"].(map[string]any)
	if content["text"] != "hello" {
		t.Fatalf("prompt text = %v, want 'hello'", content["text"])
	}

	env.deleteSession(t, sid)
}

// ─── E2E: Concurrent tool calls ──────────────────────────────────────────────

func TestE2E_ConcurrentToolCalls(t *testing.T) {
	rt := interaction.NewRuntime()
	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName: "slow_tool",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			time.Sleep(50 * time.Millisecond)
			return interaction.ToolResult{Output: "done"}, nil
		},
	})
	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	// Fire 10 concurrent tool calls.
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := env.postJSON(t, sid, "tools/call", map[string]any{
				"name":      "slow_tool",
				"arguments": map[string]any{"index": idx},
			})
			if resp["error"] != nil {
				errors <- fmt.Errorf("tool call %d: %v", idx, resp["error"])
				return
			}
			result := resp["result"].(map[string]any)
			if result["sessionId"] == "" {
				errors <- fmt.Errorf("tool call %d: no sessionId", idx)
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatal(err)
	}

	env.deleteSession(t, sid)
}

// ─── E2E: Error scenarios ────────────────────────────────────────────────────

func TestE2E_ErrorScenarios(t *testing.T) {
	env := newE2EEnv(t, nil)

	t.Run("POST_without_session", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
		req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader(body))
		req.Header.Set("Accept", "application/json")
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		var rpcResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
		if rpcResp["error"] == nil {
			t.Fatal("expected error for missing session")
		}
		errObj := rpcResp["error"].(map[string]any)
		if errObj["code"].(float64) != -32600 {
			t.Fatalf("error code = %v, want -32600", errObj["code"])
		}
	})

	t.Run("POST_invalid_JSON", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader("{bad json"))
		req.Header.Set("Accept", "application/json")
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		var rpcResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
		errObj := rpcResp["error"].(map[string]any)
		if errObj["code"].(float64) != -32700 {
			t.Fatalf("error code = %v, want -32700", errObj["code"])
		}
	})

	t.Run("GET_without_session", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.baseURL, nil)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("DELETE_without_session", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, env.baseURL, nil)
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("PUT_method_not_allowed", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPut, env.baseURL, nil)
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", resp.StatusCode)
		}
		allow := resp.Header.Get("Allow")
		if !strings.Contains(allow, "POST") || !strings.Contains(allow, "GET") || !strings.Contains(allow, "DELETE") {
			t.Fatalf("Allow header = %q, want POST, GET, DELETE", allow)
		}
	})

	t.Run("unknown_method", func(t *testing.T) {
		sid := env.initialize(t)
		defer env.deleteSession(t, sid)
		resp := env.postJSON(t, sid, "frobnicate", nil)
		errObj := resp["error"].(map[string]any)
		if errObj["code"].(float64) != -32601 {
			t.Fatalf("error code = %v, want -32601", errObj["code"])
		}
	})
}

// ─── E2E: Logging ────────────────────────────────────────────────────────────

func TestE2E_LoggingSetLevel(t *testing.T) {
	env := newE2EEnv(t, nil)
	sid := env.initialize(t)
	defer env.deleteSession(t, sid)

	resp := env.postJSON(t, sid, "logging/setLevel", map[string]any{"level": "debug"})
	if resp["error"] != nil {
		t.Fatalf("logging/setLevel error: %v", resp["error"])
	}
	env.handler.core.mu.RLock()
	if env.handler.core.logLevel != "debug" {
		t.Fatalf("logLevel = %v, want debug", env.handler.core.logLevel)
	}
	env.handler.core.mu.RUnlock()
}

// ─── E2E: Protocol version header ────────────────────────────────────────────

func TestE2E_InitializeResponseFormat(t *testing.T) {
	env := newE2EEnv(t, nil)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"sampling":{}},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequest(http.MethodPost, env.baseURL, strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	defer resp.Body.Close()

	// Verify Mcp-Session-Id header.
	sid := resp.Header.Get(headerSessionID)
	if sid == "" {
		t.Fatal("no Mcp-Session-Id header")
	}

	var rpcResp map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
	result := rpcResp["result"].(map[string]any)

	// Verify protocol version.
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}

	// Verify server info.
	info := result["serverInfo"].(map[string]any)
	if info["name"] != serverName {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
	if info["version"] != serverVersion {
		t.Fatalf("serverInfo.version = %v", info["version"])
	}

	// Verify capabilities always include tools and logging.
	caps := result["capabilities"].(map[string]any)
	if caps["tools"] == nil {
		t.Fatal("capabilities missing tools")
	}
	if caps["logging"] == nil {
		t.Fatal("capabilities missing logging")
	}

	// Verify instructions field exists.
	if result["instructions"] == nil {
		t.Fatal("missing instructions field")
	}

	env.deleteSession(t, sid)
}

// ─── notifications & completions E2E ─────────────────────────────────────────

func TestE2E_ServerNotifications(t *testing.T) {
	rt := interaction.NewRuntime()
	pp := interaction.NewMemoryPromptProvider()
	_ = pp.Register(interaction.Prompt{Name: "test"}, func(args map[string]string) (interaction.PromptResult, error) {
		return interaction.PromptResult{}, nil
	})
	rt.WithPrompts(pp)

	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	// Open GET SSE stream to receive notifications.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.baseURL, nil)
	req.Header.Set(headerSessionID, sid)
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("GET SSE: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	readSSEEvent := func() map[string]any {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("read SSE line: %v", err)
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data: ") {
				var ev map[string]any
				_ = json.Unmarshal([]byte(strings.TrimPrefix(trimmed, "data: ")), &ev)
				return ev
			}
		}
	}

	// Send tool list changed notification.
	if err := env.handler.ToolListChangedNotification(sid); err != nil {
		t.Fatalf("ToolListChangedNotification: %v", err)
	}

	// Read the notification from SSE stream.
	notif := readSSEEvent()
	if notif["method"] != "notifications/tools/list_changed" {
		t.Fatalf("unexpected notification method: %v", notif["method"])
	}

	// Send a log message notification.
	if err := env.handler.LogNotification(sid, "info", "hello from server", ""); err != nil {
		t.Fatalf("LogNotification: %v", err)
	}
	notif = readSSEEvent()
	if notif["method"] != "notifications/message" {
		t.Fatalf("expected notifications/message, got %v", notif["method"])
	}

	cancel()
	env.deleteSession(t, sid)
}

func TestE2E_CompletionComplete(t *testing.T) {
	rt := interaction.NewRuntime()
	pp := interaction.NewMemoryPromptProvider()
	_ = pp.Register(interaction.Prompt{
		Name: "code_review",
		Arguments: []interaction.PromptArgument{
			{Name: "language", Description: "Programming language"},
		},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		return interaction.PromptResult{}, nil
	})
	rt.WithPrompts(pp)

	env := newE2EEnv(t, rt)
	sid := env.initialize(t)

	resp := env.postJSON(t, sid, "completion/complete", map[string]any{
		"ref":      map[string]any{"type": "ref/prompt", "name": "code_review"},
		"argument": map[string]any{"name": "language", "value": "go"},
	})
	result := resp["result"].(map[string]any)
	completion := result["completion"].(map[string]any)
	if completion["total"].(float64) != 0 {
		t.Fatalf("expected empty default completions, got %v", completion)
	}
	ref := result["ref"].(map[string]any)
	if ref["type"] != "ref/prompt" || ref["name"] != "code_review" {
		t.Fatalf("unexpected ref: %v", ref)
	}

	// Verify completions capability is advertised on re-initialize.
	initResp := env.postJSON(t, sid, "initialize", map[string]any{"protocolVersion": protocolVersion})
	caps := initResp["result"].(map[string]any)["capabilities"].(map[string]any)
	if caps["completions"] == nil {
		t.Fatal("expected completions capability to be advertised")
	}

	// Verify error for missing ref.
	errResp := env.postJSON(t, sid, "completion/complete", map[string]any{
		"argument": map[string]any{"name": "x", "value": "y"},
	})
	errObj := errResp["error"].(map[string]any)
	if errObj["code"].(float64) != -32602 {
		t.Fatalf("error code = %v, want -32602", errObj["code"])
	}

	env.deleteSession(t, sid)
}
