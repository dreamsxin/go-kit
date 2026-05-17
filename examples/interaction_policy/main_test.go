package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	interactionmcp "github.com/dreamsxin/go-kit/interaction/mcp"
)

func TestInteractionPolicyExampleAllowsAndAuditsToolCalls(t *testing.T) {
	rt, audits := newRuntime()
	handler := interactionmcp.NewHandler(rt)

	resp := postRPC(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "echo",
			"subject":   "agent",
			"arguments": map[string]any{"message": "hello"},
			"metadata":  map[string]string{"trace": "abc"},
		},
	})
	if _, ok := resp["result"].(map[string]any)["sessionId"].(string); !ok {
		t.Fatalf("expected sessionId in response: %+v", resp)
	}

	records := audits.List()
	if len(records) != 2 {
		t.Fatalf("audit records length = %d, want 2", len(records))
	}
	if records[0].Phase != "before" || records[1].Phase != "after" {
		t.Fatalf("unexpected audit phases: %+v", records)
	}
}

func TestInteractionPolicyExampleDeniesUnknownTools(t *testing.T) {
	rt, audits := newRuntime()
	handler := interactionmcp.NewHandler(rt)

	resp := postRPC(t, handler, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":    "delete_all",
			"subject": "agent",
		},
	})
	errObj := resp["error"].(map[string]any)
	if errObj["message"] != "tool call failed" {
		t.Fatalf("unexpected error response: %+v", resp)
	}
	if len(audits.List()) != 0 {
		t.Fatalf("denied calls should not reach audit hook after authorization failure")
	}
}

func postRPC(t *testing.T, handler http.Handler, body map[string]any) map[string]any {
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
