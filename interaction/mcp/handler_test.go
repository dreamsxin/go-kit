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

func TestHandlerListsAndCallsTools(t *testing.T) {
	rt := interaction.NewRuntime(nil, nil, nil)
	if err := rt.RegisterTool(describedTool{
		ToolFunc: interaction.ToolFunc{
			ToolName: "echo",
			Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
				return interaction.ToolResult{Output: call.Input, Metadata: map[string]string{"ok": "true"}}, nil
			},
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	handler := NewHandler(rt)

	listResp := postJSON(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	tools := listResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["name"] != "echo" || tool["description"] != "Echoes the provided arguments." {
		t.Fatalf("unexpected tool descriptor: %+v", tool)
	}

	callResp := postJSON(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
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

	events, err := rt.Events.List(context.Background(), interaction.SessionID(result["sessionId"].(string)))
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events length = %d, want 3", len(events))
	}
}

func TestHandlerReturnsJSONRPCErrors(t *testing.T) {
	handler := NewHandler(interaction.NewRuntime(nil, nil, nil))

	resp := postJSON(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "missing"},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["message"] != "tool call failed" {
		t.Fatalf("error = %+v, want tool call failed", errObj)
	}
}

func TestHandlerRejectsNonPOST(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()

	NewHandler(nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func postJSON(t *testing.T, handler http.Handler, body map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	return resp
}

type describedTool struct {
	interaction.ToolFunc
}

func (d describedTool) Descriptor() interaction.ToolDescriptor {
	return interaction.ToolDescriptor{
		Name:        d.Name(),
		Description: "Echoes the provided arguments.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
}
