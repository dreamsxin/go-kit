package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// statelessRequest posts one self-describing 2026-07-28 request. It deliberately
// sets no session header: the point of every case below is that none is needed.
func statelessRequest(t *testing.T, h http.Handler, method, name string, body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	req.Header.Set(headerProtocolVersion, protocolVersion)
	req.Header.Set(headerMethod, method)
	if name != "" {
		req.Header.Set(headerName, name)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	}
	return rec, resp
}

func statelessCall(method string, params map[string]any) map[string]any {
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	return body
}

// TestStatelessRequestNeedsNoSession proves the property the whole revision
// exists for: a request carries what it needs, so any instance can answer it.
func TestStatelessRequestNeedsNoSession(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	rec, resp := statelessRequest(t, h, "tools/list", "", statelessCall("tools/list", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list without a session: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(headerProtocolVersion); got != protocolVersion {
		t.Errorf("%s = %q, want %q", headerProtocolVersion, got, protocolVersion)
	}
	if got := rec.Header().Get(headerSessionID); got != "" {
		t.Errorf("%s = %q, want no session header on the stateless path", headerSessionID, got)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result missing: %s", rec.Body.String())
	}
	if tools, ok := result["tools"].([]any); !ok || len(tools) == 0 {
		t.Fatalf("tools = %v, want the registered tool", result["tools"])
	}

	// A tool call is the case that used to need a session for its runtime
	// bookkeeping, so it is worth its own assertion.
	rec, resp = statelessRequest(t, h, "tools/call", "echo",
		statelessCall("tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"message": "hi"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call without a session: status=%d body=%s", rec.Code, rec.Body.String())
	}
	result, _ = resp["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("tools/call result = %v", result)
	}
}

// TestStatelessRetiresTheHandshake proves the handshake is gone on the new
// revision rather than merely unnecessary, and that the error says where to go.
func TestStatelessRetiresTheHandshake(t *testing.T) {
	h := NewStreamableHandler(nil)
	defer h.Close() //nolint:errcheck

	_, resp := statelessRequest(t, h, "initialize", "",
		statelessCall("initialize", map[string]any{"protocolVersion": protocolVersion}))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("initialize answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32601 {
		t.Errorf("initialize error code = %v, want -32601", rpcErr["code"])
	}
	if data, _ := rpcErr["data"].(string); !contains(data, "server/discover") {
		t.Errorf("initialize error data = %q, want it to name server/discover", data)
	}

	// The matching notification is not an error worth reporting: a client that
	// sends it has nothing to fix in this response.
	rec, _ := statelessRequest(t, h, "notifications/initialized", "",
		map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if rec.Code != http.StatusAccepted {
		t.Errorf("notifications/initialized: status=%d, want 202", rec.Code)
	}
}

// TestStatelessServerDiscover proves the one call that replaces the handshake
// reports everything a client used to learn from it.
func TestStatelessServerDiscover(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	rec, resp := statelessRequest(t, h, "server/discover", "", statelessCall("server/discover", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("server/discover: status=%d body=%s", rec.Code, rec.Body.String())
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("server/discover result missing: %s", rec.Body.String())
	}
	if result["resultType"] != "complete" {
		t.Errorf("resultType = %v, want complete", result["resultType"])
	}
	versions, _ := result["supportedVersions"].([]any)
	if len(versions) != 2 || versions[0] != protocolVersion || versions[1] != legacyProtocolVersion {
		t.Errorf("supportedVersions = %v, want [%s %s]", versions, protocolVersion, legacyProtocolVersion)
	}
	caps, _ := result["capabilities"].(map[string]any)
	for _, key := range []string{"tools", "resources", "prompts"} {
		if _, ok := caps[key]; !ok {
			t.Errorf("capabilities missing %s: %v", key, caps)
		}
	}
	if _, ok := caps["logging"]; ok {
		t.Errorf("capabilities advertise logging, which needs the session this revision does not have: %v", caps)
	}
	meta, _ := result["_meta"].(map[string]any)
	info, _ := meta[metaServerInfo].(map[string]any)
	if info["name"] != serverName {
		t.Errorf("%s.name = %v, want %s", metaServerInfo, info["name"], serverName)
	}
	if result["ttlMs"] == nil || result["cacheScope"] == nil {
		t.Errorf("server/discover carries no cache hints: %v", result)
	}
}

// TestStatelessRequiresRoutingHeaders proves the headers a gateway routes,
// meters and authorizes on describe the body it forwarded. A request whose
// header says one thing and whose body does another is the case that would let a
// per-tool rate limit or policy be bypassed.
func TestStatelessRequiresRoutingHeaders(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	post := func(headers map[string]string, body map[string]any) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
		req.Header.Set(headerProtocolVersion, protocolVersion)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	cases := []struct {
		name    string
		headers map[string]string
		body    map[string]any
	}{
		{
			name:    "MethodHeaderMissing",
			headers: nil,
			body:    statelessCall("tools/list", nil),
		},
		{
			name:    "MethodHeaderDisagreesWithBody",
			headers: map[string]string{headerMethod: "tools/list"},
			body:    statelessCall("tools/call", map[string]any{"name": "echo"}),
		},
		{
			name:    "NameHeaderMissingForATargetedMethod",
			headers: map[string]string{headerMethod: "tools/call"},
			body:    statelessCall("tools/call", map[string]any{"name": "echo"}),
		},
		{
			name:    "NameHeaderDisagreesWithBody",
			headers: map[string]string{headerMethod: "tools/call", headerName: "delete_everything"},
			body:    statelessCall("tools/call", map[string]any{"name": "echo"}),
		},
		{
			name:    "NameHeaderOnAMethodWithNoTarget",
			headers: map[string]string{headerMethod: "tools/list", headerName: "echo"},
			body:    statelessCall("tools/list", nil),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(tc.headers, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400: %s", rec.Code, rec.Body.String())
			}
			var body map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if body["error"] != "invalid_routing_header" {
				t.Fatalf("error = %q, want invalid_routing_header", body["error"])
			}
		})
	}

	rec := post(map[string]string{headerMethod: "tools/call", headerName: "echo"},
		statelessCall("tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"message": "hi"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("agreeing headers: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestStatelessListResultsCarryCacheHints proves a client can cache a catalog:
// without ttlMs and cacheScope it has to re-fetch on every reconnect, which is
// what the removed listChanged stream used to cover.
func TestStatelessListResultsCarryCacheHints(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	for _, method := range []string{"tools/list", "prompts/list", "resources/list", "resources/templates/list"} {
		_, resp := statelessRequest(t, h, method, "", statelessCall(method, nil))
		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("%s result missing: %v", method, resp)
		}
		if ttl, _ := result["ttlMs"].(float64); ttl != float64(defaultListCacheTTL.Milliseconds()) {
			t.Errorf("%s ttlMs = %v, want %d", method, result["ttlMs"], defaultListCacheTTL.Milliseconds())
		}
		if result["cacheScope"] != defaultListCacheScope {
			t.Errorf("%s cacheScope = %v, want %s", method, result["cacheScope"], defaultListCacheScope)
		}
	}

	// resources/read is cacheable too, and it is the one that carries content
	// rather than a catalog.
	_, resp := statelessRequest(t, h, "resources/read", "config://app/name",
		statelessCall("resources/read", map[string]any{"uri": "config://app/name"}))
	result, _ := resp["result"].(map[string]any)
	if result["ttlMs"] == nil || result["cacheScope"] == nil {
		t.Errorf("resources/read carries no cache hints: %v", result)
	}

	h.ListCacheTTL = 5 * time.Second
	h.ListCacheScope = "public"
	_, resp = statelessRequest(t, h, "tools/list", "", statelessCall("tools/list", nil))
	result, _ = resp["result"].(map[string]any)
	if ttl, _ := result["ttlMs"].(float64); ttl != 5000 {
		t.Errorf("configured ttlMs = %v, want 5000", result["ttlMs"])
	}
	if result["cacheScope"] != "public" {
		t.Errorf("configured cacheScope = %v, want public", result["cacheScope"])
	}
}

// TestStatelessCarriesClientIdentity proves the identity a session used to hold
// reaches a tool, and that a request contradicting its own header is refused.
func TestStatelessCarriesClientIdentity(t *testing.T) {
	rt := interaction.NewRuntime()
	var seenName, seenVersion string
	var seenStateless, seenSampling bool
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName: "identity",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			seenName, seenVersion, seenStateless = IdentityFromContext(ctx)
			seenSampling = ClientCapabilityFromContext(ctx, "sampling")
			return interaction.ToolResult{Output: "ok"}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	defer h.Close() //nolint:errcheck

	rec, _ := statelessRequest(t, h, "tools/call", "identity", statelessCall("tools/call", map[string]any{
		"name": "identity",
		"_meta": map[string]any{
			metaProtocolVersion:    protocolVersion,
			metaClientInfo:         map[string]any{"name": "curl-client", "version": "1.0.0"},
			metaClientCapabilities: map[string]any{"sampling": map[string]any{}},
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/call: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !seenStateless {
		t.Fatal("the tool did not see a stateless request")
	}
	if seenName != "curl-client" || seenVersion != "1.0.0" {
		t.Errorf("client identity = %q/%q, want curl-client/1.0.0", seenName, seenVersion)
	}
	if !seenSampling {
		t.Error("the tool did not see the declared sampling capability")
	}

	_, resp := statelessRequest(t, h, "tools/list", "", statelessCall("tools/list", map[string]any{
		"_meta": map[string]any{metaProtocolVersion: "2025-06-18"},
	}))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("a request contradicting its own version header answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32600 {
		t.Errorf("error code = %v, want -32600", rpcErr["code"])
	}
}

// TestStatelessServesOnlyPOST proves GET and DELETE are gone with the sessions
// they existed for, rather than answering with a session error a client cannot act on.
func TestStatelessServesOnlyPOST(t *testing.T) {
	h := NewStreamableHandler(nil)
	defer h.Close() //nolint:errcheck

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Header.Set(headerProtocolVersion, protocolVersion)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status=%d, want 405", method, rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow != "POST" {
			t.Errorf("%s: Allow = %q, want POST", method, allow)
		}
	}
}

// TestUnknownProtocolVersionNamesBothRevisions proves the negotiation failure
// tells a client what it can speak instead.
func TestUnknownProtocolVersionNamesBothRevisions(t *testing.T) {
	h := NewStreamableHandler(nil)
	defer h.Close() //nolint:errcheck

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)))
	req.Header.Set(headerProtocolVersion, "2024-11-05")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, protocolVersion) || !contains(body, legacyProtocolVersion) {
		t.Fatalf("body = %s, want both supported revisions named", body)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && bytes.Contains([]byte(haystack), []byte(needle))
}

// askingTool needs one answer before it deletes anything, which is the shape
// every mid-call question has: run, check what arrived, ask or proceed.
func askingTool(seen *map[string]any) interaction.ToolFunc {
	return interaction.ToolFunc{
		ToolName: "delete_rows",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			answers, state := interaction.InputAnswersFromContext(ctx)
			*seen = state
			if answers["confirm"] != true {
				return interaction.ToolResult{}, &interaction.InputRequired{
					Requests: map[string]interaction.InputRequest{
						"confirm": {Kind: "elicitation", Params: map[string]any{"message": "Delete 42 rows?"}},
					},
					State: map[string]any{"rows": 42},
				}
			}
			return interaction.ToolResult{Output: "deleted"}, nil
		},
	}
}

// TestStatelessToolAsksForInputAndResumes proves a tool can ask the caller
// something without a stream to ask over: the question is a result, and the
// answer arrives as the next call's input.
func TestStatelessToolAsksForInputAndResumes(t *testing.T) {
	var seenState map[string]any
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(askingTool(&seenState)); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	defer h.Close() //nolint:errcheck

	meta := map[string]any{metaClientCapabilities: map[string]any{"elicitation": map[string]any{}}}
	_, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":  "delete_rows",
		"_meta": meta,
	}))
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("first call answered %v, want an interim result", resp)
	}
	if result["resultType"] != "input_required" {
		t.Fatalf("resultType = %v, want input_required", result["resultType"])
	}
	requests, _ := result["inputRequests"].(map[string]any)
	confirm, _ := requests["confirm"].(map[string]any)
	if confirm["kind"] != "elicitation" {
		t.Errorf("inputRequests.confirm = %v, want an elicitation", requests["confirm"])
	}
	state, _ := result["requestState"].(string)
	if state == "" {
		t.Fatal("the interim result carries no requestState, so the tool cannot resume")
	}

	_, resp = statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":           "delete_rows",
		"_meta":          meta,
		"inputResponses": map[string]any{"confirm": true},
		"requestState":   state,
	}))
	result, ok = resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("the resumed call answered %v, want a result", resp)
	}
	if result["resultType"] != "complete" {
		t.Errorf("resumed resultType = %v, want complete", result["resultType"])
	}
	if result["isError"] != false {
		t.Errorf("resumed result = %v, want a successful call", result)
	}
	if rows, _ := seenState["rows"].(float64); rows != 42 {
		t.Errorf("the tool resumed with state %v, want the 42 rows it recorded", seenState)
	}
}

// TestStatelessInputRequiresADeclaredCapability proves the server does not ask a
// question the caller never said it could answer, which would hang a client that
// has no code for it.
func TestStatelessInputRequiresADeclaredCapability(t *testing.T) {
	var seenState map[string]any
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(askingTool(&seenState)); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	defer h.Close() //nolint:errcheck

	_, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name": "delete_rows",
	}))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("a question the client cannot answer produced %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32021 {
		t.Errorf("error code = %v, want -32021", rpcErr["code"])
	}
	data, _ := rpcErr["data"].(map[string]any)
	required, _ := data["requiredCapabilities"].([]any)
	if len(required) != 1 || required[0] != "elicitation" {
		t.Errorf("data.requiredCapabilities = %v, want [elicitation]", data["requiredCapabilities"])
	}
}

// TestLegacyPathRefusesAnInputRequiredResult proves the interim result is not
// emitted where it does not exist: a 2025-06-18 client would not know to answer
// it, and its own way to ask is the session stream.
func TestLegacyPathRefusesAnInputRequiredResult(t *testing.T) {
	var seenState map[string]any
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(askingTool(&seenState)); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	defer h.Close() //nolint:errcheck

	sid := initSessionHelper(t, h)
	_, resp := streamPostJSON(t, h, sid, "tools/call", map[string]any{"name": "delete_rows"})
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("the handshake revision answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32603 {
		t.Errorf("error code = %v, want -32603", rpcErr["code"])
	}
	if data, _ := rpcErr["data"].(string); !contains(data, protocolVersion) {
		t.Errorf("error data = %q, want it to name %s", data, protocolVersion)
	}
}
