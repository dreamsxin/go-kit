package mcp

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"
)

// TestInvalidCursorIsInvalidParams covers the half of mcp.error-codes that the
// pagination helper used to swallow. A cursor is the offset this server handed
// out, so one that is not a number, is negative, or is at or past the end is one
// it never issued — and treating it as offset 0 quietly handed the caller page
// one as though it were its page.
func TestInvalidCursorIsInvalidParams(t *testing.T) {
	methods := []string{"tools/list", "resources/list", "resources/templates/list", "prompts/list"}
	cursors := []string{"not-a-number", "-1", "999999", ""}

	for _, method := range methods {
		for _, cursor := range cursors {
			if cursor == "" {
				continue // an absent cursor is the first page, not an error
			}
			t.Run(method+"/"+cursor, func(t *testing.T) {
				h := NewStreamableHandler(setupRuntime(t))
				defer h.Close() //nolint:errcheck

				_, resp := statelessRequest(t, h, method, "", statelessCall(method, map[string]any{"cursor": cursor}))
				rpcErr, ok := resp["error"].(map[string]any)
				if !ok {
					t.Fatalf("%s with cursor %q answered %v, want an error", method, cursor, resp)
				}
				if code, _ := rpcErr["code"].(float64); code != -32602 {
					t.Fatalf("code = %v, want -32602", rpcErr["code"])
				}
			})
		}
	}
}

// TestFirstPageNeedsNoCursor guards the neighbour: making a bad cursor an error
// must not make the absence of one an error too.
func TestFirstPageNeedsNoCursor(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	_, resp := statelessRequest(t, h, "tools/list", "", statelessCall("tools/list", nil))
	if _, ok := resp["result"].(map[string]any); !ok {
		t.Fatalf("tools/list without a cursor answered %v, want a result", resp)
	}
}

// unserialisableResult is what a tool returns when it hands back a value
// encoding/json cannot represent.
type unserialisableResult struct{}

func (unserialisableResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(math.NaN())
}

// TestUnserialisableResultIsAnsweredAsAnInternalError proves a response is whole
// or an error. Encoding straight to the ResponseWriter meant a value that failed
// part-way through had already put bytes on the wire, so the caller received an
// incomplete document with nothing to say what happened — or, on the SSE path, an
// empty event and a wait for a reply that never came.
func TestUnserialisableResultIsAnsweredAsAnInternalError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeResponse(recorder, response{
		JSONRPC: jsonRPCVersion,
		ID:      json.RawMessage(`7`),
		Result:  unserialisableResult{},
	})

	var answered map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the body is not valid JSON (%v): %q", err, recorder.Body.String())
	}
	rpcErr, ok := answered["error"].(map[string]any)
	if !ok {
		t.Fatalf("answered %v, want an error", answered)
	}
	if code, _ := rpcErr["code"].(float64); code != -32603 {
		t.Fatalf("code = %v, want -32603", rpcErr["code"])
	}
	if answered["id"] == nil {
		t.Error("the error answers no id, so the caller cannot match it to its request")
	}
}
