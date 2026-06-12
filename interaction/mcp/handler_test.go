package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/interaction"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func postJSON(t *testing.T, handler http.Handler, body map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func postJSONRaw(t *testing.T, handler http.Handler, body map[string]any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

// initSessionHelper sends an initialize request and returns the session ID.
func initSessionHelper(t *testing.T, handler http.Handler) string {
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

// postJSONSession sends a POST request with a session ID header.
func postJSONSession(t *testing.T, handler http.Handler, sid string, body map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// postJSONSessionRaw sends a POST request with session ID and returns status + body.
func postJSONSessionRaw(t *testing.T, handler http.Handler, sid string, body map[string]any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return rec.Code, resp
}

func setupRuntime(t *testing.T) *interaction.Runtime {
	t.Helper()
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName:    "echo",
		Description: "Echoes the provided arguments.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: call.Input, Metadata: map[string]string{"ok": "true"}}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	resources := interaction.NewMemoryResourceProvider()
	_ = resources.Register(interaction.Resource{
		URI:         "config://app/name",
		Name:        "app-name",
		Title:       "Application Name",
		Description: "The running application name",
		MIMEType:    "text/plain",
	}, []interaction.ResourceContent{{URI: "config://app/name", Text: "go-kit-demo", MIMEType: "text/plain"}})
	_ = resources.Register(interaction.Resource{
		URI:  "config://app/version",
		Name: "app-version",
	}, []interaction.ResourceContent{{URI: "config://app/version", Text: "0.1.0"}})
	resources.SetTemplates([]interaction.ResourceTemplate{{
		URITemplate: "config://{key}",
		Name:        "config-entry",
		Description: "Read a configuration value by key",
	}})
	rt.WithResources(resources)

	prompts := interaction.NewMemoryPromptProvider()
	_ = prompts.Register(interaction.Prompt{
		Name:        "code_review",
		Title:       "Code Review",
		Description: "Request a code review from the LLM",
		Arguments: []interaction.PromptArgument{
			{Name: "code", Description: "The source code to review", Required: true},
			{Name: "language", Description: "Programming language"},
		},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		lang := args["language"]
		if lang == "" {
			lang = "unknown"
		}
		return interaction.PromptResult{
			Description: "Code review prompt",
			Messages: []interaction.PromptMessage{
				{Role: "user", Content: interaction.PromptContent{
					Type: "text",
					Text: "Review this " + lang + " code:\n" + args["code"],
				}},
			},
		}, nil
	})
	rt.WithPrompts(prompts)

	return rt
}

// ─── initialize ──────────────────────────────────────────────────────────────

func TestInitialize(t *testing.T) {
	rt := setupRuntime(t)
	handler := NewHandler(rt)

	resp := postJSON(t, handler, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	})
	result := resp["result"].(map[string]any)

	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", result["protocolVersion"], protocolVersion)
	}
	info := result["serverInfo"].(map[string]any)
	if info["name"] != serverName {
		t.Fatalf("serverInfo.name = %v, want %s", info["name"], serverName)
	}
	caps := result["capabilities"].(map[string]any)
	for _, key := range []string{"tools", "resources", "prompts", "logging"} {
		if _, ok := caps[key]; !ok {
			t.Fatalf("missing capability: %s", key)
		}
	}
}

func TestInitializeWithoutOptionalProviders(t *testing.T) {
	rt := interaction.NewRuntime()
	handler := NewHandler(rt)

	resp := postJSON(t, handler, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
	})
	result := resp["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)

	if _, ok := caps["tools"]; !ok {
		t.Fatal("tools capability should always be present")
	}
	if _, ok := caps["resources"]; ok {
		t.Fatal("resources capability should not be present without a ResourceProvider")
	}
	if _, ok := caps["prompts"]; ok {
		t.Fatal("prompts capability should not be present without a PromptProvider")
	}
}

// ─── ping ────────────────────────────────────────────────────────────────────

func TestPing(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "ping",
	})
	result := resp["result"].(map[string]any)
	if len(result) != 0 {
		t.Fatalf("ping result should be empty object, got %v", result)
	}
}

// ─── notifications/initialized ───────────────────────────────────────────────

func TestNotificationsInitialized(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)

	body := map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerSessionID, sid)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Verify session is marked as initialized.
	sess, ok := handler.store.get(sid)
	if !ok {
		t.Fatal("session not found after initialized notification")
	}
	sess.mu.RLock()
	if !sess.initialized {
		t.Fatal("session should be marked initialized after notifications/initialized")
	}
	sess.mu.RUnlock()
}

// ─── tools ───────────────────────────────────────────────────────────────────

func TestHandlerListsAndCallsTools(t *testing.T) {
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName:    "echo",
		Description: "Echoes the provided arguments.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{Output: call.Input, Metadata: map[string]string{"ok": "true"}}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	handler := NewHandler(rt)
	sid := initSessionHelper(t, handler)

	listResp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	tools := listResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "echo" || tool["description"] != "Echoes the provided arguments." {
		t.Fatalf("unexpected tool descriptor: %+v", tool)
	}

	callResp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": "call-1", "method": "tools/call",
		"params": map[string]any{
			"name":      "echo",
			"arguments": map[string]any{"message": "hello"},
			"metadata":  map[string]string{"trace": "abc"},
		},
	})
	result := callResp["result"].(map[string]any)
	if result["sessionId"] == "" {
		t.Fatalf("sessionId should be returned: %+v", result)
	}
	if result["metadata"].(map[string]any)["ok"] != "true" {
		t.Fatalf("metadata = %+v, want ok=true", result["metadata"])
	}
}

func TestHandlerReturnsJSONRPCErrors(t *testing.T) {
	handler := NewHandler(interaction.NewRuntime())
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "missing"},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["message"] != "tool call failed" {
		t.Fatalf("error = %+v, want tool call failed", errObj)
	}
}

func TestHandlerRejectsPut(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/mcp", nil)
	rec := httptest.NewRecorder()
	NewHandler(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ─── resources ───────────────────────────────────────────────────────────────

func TestResourcesList(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "resources/list",
	})
	result := resp["result"].(map[string]any)
	resources := result["resources"].([]any)
	if len(resources) != 2 {
		t.Fatalf("resources length = %d, want 2", len(resources))
	}
	first := resources[0].(map[string]any)
	if first["uri"] != "config://app/name" || first["name"] != "app-name" {
		t.Fatalf("unexpected resource: %+v", first)
	}
}

func TestResourcesListEmpty(t *testing.T) {
	handler := NewHandler(interaction.NewRuntime())
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "resources/list",
	})
	result := resp["result"].(map[string]any)
	resources := result["resources"].([]any)
	if len(resources) != 0 {
		t.Fatalf("resources length = %d, want 0", len(resources))
	}
}

func TestResourcesRead(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "resources/read",
		"params": map[string]any{"uri": "config://app/name"},
	})
	result := resp["result"].(map[string]any)
	contents := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents length = %d, want 1", len(contents))
	}
	c := contents[0].(map[string]any)
	if c["text"] != "go-kit-demo" {
		t.Fatalf("text = %v, want go-kit-demo", c["text"])
	}
}

func TestResourcesReadNotFound(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "resources/read",
		"params": map[string]any{"uri": "config://missing"},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["code"].(float64) != -32002 {
		t.Fatalf("error code = %v, want -32002", errObj["code"])
	}
}

func TestResourceTemplatesList(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "resources/templates/list",
	})
	result := resp["result"].(map[string]any)
	templates := result["resourceTemplates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("templates length = %d, want 1", len(templates))
	}
	tpl := templates[0].(map[string]any)
	if tpl["uriTemplate"] != "config://{key}" {
		t.Fatalf("uriTemplate = %v, want config://{key}", tpl["uriTemplate"])
	}
}

// ─── prompts ─────────────────────────────────────────────────────────────────

func TestPromptsList(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "prompts/list",
	})
	result := resp["result"].(map[string]any)
	prompts := result["prompts"].([]any)
	if len(prompts) != 1 {
		t.Fatalf("prompts length = %d, want 1", len(prompts))
	}
	p := prompts[0].(map[string]any)
	if p["name"] != "code_review" {
		t.Fatalf("name = %v, want code_review", p["name"])
	}
	args := p["arguments"].([]any)
	if len(args) != 2 {
		t.Fatalf("arguments length = %d, want 2", len(args))
	}
}

func TestPromptsGet(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "prompts/get",
		"params": map[string]any{
			"name":      "code_review",
			"arguments": map[string]string{"code": "print('hi')", "language": "python"},
		},
	})
	result := resp["result"].(map[string]any)
	if result["description"] != "Code review prompt" {
		t.Fatalf("description = %v, want Code review prompt", result["description"])
	}
	messages := result["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("messages length = %d, want 1", len(messages))
	}
	msg := messages[0].(map[string]any)
	if msg["role"] != "user" {
		t.Fatalf("role = %v, want user", msg["role"])
	}
	content := msg["content"].(map[string]any)
	if content["type"] != "text" {
		t.Fatalf("content type = %v, want text", content["type"])
	}
	if content["text"] != "Review this python code:\nprint('hi')" {
		t.Fatalf("content text = %v", content["text"])
	}
}

func TestPromptsGetNotFound(t *testing.T) {
	handler := NewHandler(setupRuntime(t))
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "prompts/get",
		"params": map[string]any{"name": "nonexistent"},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["code"].(float64) != -32602 {
		t.Fatalf("error code = %v, want -32602", errObj["code"])
	}
}

func TestPromptsListEmpty(t *testing.T) {
	handler := NewHandler(interaction.NewRuntime())
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "prompts/list",
	})
	result := resp["result"].(map[string]any)
	prompts := result["prompts"].([]any)
	if len(prompts) != 0 {
		t.Fatalf("prompts length = %d, want 0", len(prompts))
	}
}

// ─── logging ─────────────────────────────────────────────────────────────────

func TestLoggingSetLevel(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "logging/setLevel",
		"params": map[string]any{"level": "debug"},
	})
	if resp["error"] != nil {
		t.Fatalf("unexpected error: %+v", resp["error"])
	}
	handler.core.mu.RLock()
	if handler.core.logLevel != "debug" {
		t.Fatalf("logLevel = %v, want debug", handler.core.logLevel)
	}
	handler.core.mu.RUnlock()
}

func TestLoggingSetLevelInvalid(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "logging/setLevel",
		"params": map[string]any{"level": "bogus"},
	})
	if resp["error"] == nil {
		t.Fatal("expected JSON-RPC error for invalid log level")
	}
	handler.core.mu.RLock()
	if handler.core.logLevel != "info" {
		t.Fatalf("logLevel = %v, want info (unchanged)", handler.core.logLevel)
	}
	handler.core.mu.RUnlock()
}

// ─── pagination ──────────────────────────────────────────────────────────────

func TestToolsListPagination(t *testing.T) {
	rt := interaction.NewRuntime()
	for i := 0; i < 5; i++ {
		name := "tool_" + string(rune('a'+i))
		_ = rt.RegisterTool(interaction.ToolFunc{
			ToolName: name,
			Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
				return interaction.ToolResult{}, nil
			},
		})
	}

	// Override defaultPageSize for testing
	origPageSize := defaultPageSize
	defer func() { /* defaultPageSize is const, so we test via cursor instead */ }()
	_ = origPageSize

	handler := NewHandler(rt)
	sid := initSessionHelper(t, handler)

	// First page: no cursor
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 5 {
		t.Fatalf("tools length = %d, want 5", len(tools))
	}
	if result["nextCursor"] != nil {
		t.Fatalf("nextCursor should be nil for single page, got %v", result["nextCursor"])
	}
}

// ─── unknown method ──────────────────────────────────────────────────────────

func TestUnknownMethod(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "frobnicate",
	})
	errObj := resp["error"].(map[string]any)
	if errObj["code"].(float64) != -32601 {
		t.Fatalf("error code = %v, want -32601", errObj["code"])
	}
}

// ─── completions ─────────────────────────────────────────────────────────────

func TestCompletionComplete_Prompt(t *testing.T) {
	rt := interaction.NewRuntime()
	pp := interaction.NewMemoryPromptProvider()
	_ = pp.Register(interaction.Prompt{
		Name:        "summarize",
		Description: "Summarize text",
		Arguments: []interaction.PromptArgument{
			{Name: "style", Description: "Style of summary"},
		},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		return interaction.PromptResult{}, nil
	})
	rt.WithPrompts(pp)

	handler := NewHandler(rt)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "completion/complete",
		"params": map[string]any{
			"ref": map[string]any{"type": "ref/prompt", "name": "summarize"},
			"argument": map[string]any{"name": "style", "value": "bul"},
		},
	})
	result := resp["result"].(map[string]any)
	completion := result["completion"].(map[string]any)
	if completion["total"].(float64) != 0 {
		t.Fatalf("expected empty completions by default, got %v", completion)
	}
}

func TestCompletionComplete_UnsupportedRefType(t *testing.T) {
	rt := interaction.NewRuntime()
	handler := NewHandler(rt)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "completion/complete",
		"params": map[string]any{
			"ref":      map[string]any{"type": "ref/resource", "name": "foo"},
			"argument": map[string]any{"name": "bar", "value": "baz"},
		},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["code"].(float64) != -32602 {
		t.Fatalf("error code = %v, want -32602", errObj["code"])
	}
}

func TestCompletionComplete_MissingRef(t *testing.T) {
	handler := NewHandler(nil)
	sid := initSessionHelper(t, handler)
	resp := postJSONSession(t, handler, sid, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "completion/complete",
		"params":  map[string]any{"argument": map[string]any{"name": "x", "value": "y"}},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["code"].(float64) != -32602 {
		t.Fatalf("error code = %v, want -32602", errObj["code"])
	}
}
