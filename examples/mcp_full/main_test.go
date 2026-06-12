package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	interactionmcp "github.com/dreamsxin/go-kit/interaction/mcp"
)

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
	sid := rec.Header().Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatal("initialize: no Mcp-Session-Id header")
	}
	// Consume the body so the response is fully read.
	_ = rec.Body
	return sid
}

func postMCP(t *testing.T, handler http.Handler, sid string, method string, params any) map[string]any {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set("Mcp-Session-Id", sid)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	return resp
}

func TestMCPFullExample(t *testing.T) {
	rt := buildRuntime()
	handler := interactionmcp.NewHandler(rt)

	// Initialize — get session ID.
	sid := initSession(t, handler)

	// Initialize response should declare tools, resources, prompts, and logging.
	initBody := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	payload, _ := json.Marshal(initBody)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var initResp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&initResp)
	result := initResp["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)
	for _, key := range []string{"tools", "resources", "prompts", "logging"} {
		if _, ok := caps[key]; !ok {
			t.Fatalf("missing capability: %s", key)
		}
	}

	// Tools list should contain greet and current_time.
	toolsResp := postMCP(t, handler, sid, "tools/list", nil)
	tools := toolsResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools length = %d, want 2", len(tools))
	}

	// Call greet tool.
	greetResp := postMCP(t, handler, sid, "tools/call", map[string]any{
		"name":      "greet",
		"arguments": map[string]any{"name": "Alice", "style": "formal"},
	})
	greetResult := greetResp["result"].(map[string]any)
	content := greetResult["content"].([]any)[0].(map[string]any)
	if content["text"] == "" {
		t.Fatal("greet tool returned empty text")
	}

	// Resources list should contain 3 resources.
	resResp := postMCP(t, handler, sid, "resources/list", nil)
	resources := resResp["result"].(map[string]any)["resources"].([]any)
	if len(resources) != 3 {
		t.Fatalf("resources length = %d, want 3", len(resources))
	}

	// Read a resource.
	readResp := postMCP(t, handler, sid, "resources/read", map[string]any{"uri": "info://app/name"})
	contents := readResp["result"].(map[string]any)["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("contents length = %d, want 1", len(contents))
	}
	if contents[0].(map[string]any)["text"] != "go-kit MCP Demo" {
		t.Fatalf("resource text = %v", contents[0])
	}

	// Resource templates.
	tplResp := postMCP(t, handler, sid, "resources/templates/list", nil)
	templates := tplResp["result"].(map[string]any)["resourceTemplates"].([]any)
	_ = templates // may be empty for this example

	// Prompts list should contain summarize and code_review.
	promptsResp := postMCP(t, handler, sid, "prompts/list", nil)
	prompts := promptsResp["result"].(map[string]any)["prompts"].([]any)
	if len(prompts) != 2 {
		t.Fatalf("prompts length = %d, want 2", len(prompts))
	}

	// Get a prompt.
	promptResp := postMCP(t, handler, sid, "prompts/get", map[string]any{
		"name":      "code_review",
		"arguments": map[string]string{"code": "fmt.Println(\"hi\")", "language": "go"},
	})
	promptResult := promptResp["result"].(map[string]any)
	messages := promptResult["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want 2 (system + user)", len(messages))
	}

	// Ping.
	pingResp := postMCP(t, handler, sid, "ping", nil)
	if pingResp["result"].(map[string]any) == nil {
		t.Fatal("ping should return empty result")
	}

	// Logging.
	logResp := postMCP(t, handler, sid, "logging/setLevel", map[string]any{"level": "debug"})
	if logResp["error"] != nil {
		t.Fatalf("logging/setLevel error: %v", logResp["error"])
	}
}
