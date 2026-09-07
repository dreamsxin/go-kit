package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamsxin/go-kit/v2/interaction"
	"github.com/dreamsxin/go-kit/v2/security"
)

// recordAuthorizations returns an authorizer that records what it was asked and
// answers with err.
func recordAuthorizations(seen *[]MethodRequest, err error) MethodAuthorizerFunc {
	return func(_ context.Context, req MethodRequest) error {
		*seen = append(*seen, req)
		return err
	}
}

// TestMethodAuthorizerSeesEveryRequest proves the seam covers the whole method
// surface, not just tool calls: a deployment that refuses "resources/read" for
// an anonymous caller has somewhere to say so.
func TestMethodAuthorizerSeesEveryRequest(t *testing.T) {
	var seen []MethodRequest
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	h.Authorizer = recordAuthorizations(&seen, nil)

	cases := []struct {
		method string
		target string
		params map[string]any
	}{
		{method: "ping"},
		{method: "server/discover"},
		{method: "tools/list"},
		{method: "tools/call", target: "echo", params: map[string]any{"name": "echo", "arguments": map[string]any{"message": "hi"}}},
		{method: "resources/list"},
		{method: "resources/read", target: "config://app/name", params: map[string]any{"uri": "config://app/name"}},
		{method: "resources/templates/list"},
		{method: "prompts/list"},
		{method: "prompts/get", target: "code_review", params: map[string]any{"name": "code_review", "arguments": map[string]any{"code": "x := 1"}}},
		{method: "completion/complete", params: map[string]any{
			"ref":      map[string]any{"type": "ref/prompt", "name": "code_review"},
			"argument": map[string]any{"name": "language", "value": "g"},
		}},
	}
	for _, tc := range cases {
		seen = nil
		rec, _ := statelessRequest(t, h, tc.method, tc.target, statelessCall(tc.method, tc.params))
		if len(seen) != 1 {
			t.Fatalf("%s reached the authorizer %d times, want once (status=%d body=%s)",
				tc.method, len(seen), rec.Code, rec.Body.String())
		}
		got := seen[0]
		if got.Method != tc.method {
			t.Errorf("%s: authorized method = %q", tc.method, got.Method)
		}
		if got.Target != tc.target {
			t.Errorf("%s: authorized target = %q, want %q", tc.method, got.Target, tc.target)
		}
		if got.ProtocolVersion != protocolVersion {
			t.Errorf("%s: authorized revision = %q, want %q", tc.method, got.ProtocolVersion, protocolVersion)
		}
		if got.SessionID != "" {
			t.Errorf("%s: authorized session = %q, want none on the stateless revision", tc.method, got.SessionID)
		}
	}
}

// TestMethodAuthorizerRefusalStopsDispatch proves a refusal is answered as
// unauthorized and that nothing ran behind it — a refusal that still executed
// the tool would be an audit trail, not a policy.
func TestMethodAuthorizerRefusalStopsDispatch(t *testing.T) {
	rt := interaction.NewRuntime()
	called := false
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName: "echo",
		Fn: func(context.Context, interaction.ToolCall) (interaction.ToolResult, error) {
			called = true
			return interaction.ToolResult{Output: "ok"}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	defer h.Close() //nolint:errcheck

	cases := []struct {
		name string
		err  error
		code float64
	}{
		{name: "NamedRefusal", err: fmt.Errorf("%w: tool echo is operator-only", interaction.ErrUnauthorized), code: -32001},
		{name: "UnwrappedRefusal", err: errors.New("policy service said no"), code: -32001},
		{name: "RefusalThatNamesAnotherFailure", err: fmt.Errorf("%w: tenant header is required", interaction.ErrInvalidArgument), code: -32602},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			var seen []MethodRequest
			h.Authorizer = recordAuthorizations(&seen, tc.err)

			rec, resp := statelessRequest(t, h, "tools/call", "echo",
				statelessCall("tools/call", map[string]any{"name": "echo"}))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want the refusal in the JSON-RPC body", rec.Code)
			}
			rpcErr, ok := resp["error"].(map[string]any)
			if !ok {
				t.Fatalf("a refused call answered %v, want an error", resp)
			}
			if code, _ := rpcErr["code"].(float64); code != tc.code {
				t.Errorf("error code = %v, want %v", rpcErr["code"], tc.code)
			}
			if data, _ := rpcErr["data"].(string); !contains(data, tc.err.Error()) {
				t.Errorf("error data = %q, want the authorizer's reason", data)
			}
			if called {
				t.Error("the tool ran despite the refusal")
			}
		})
	}
}

// TestWithoutAnAuthorizerEveryMethodIsServed proves the framework invents no
// policy of its own: authorization is opt-in because only the deployment knows
// what its callers are allowed to do.
func TestWithoutAnAuthorizerEveryMethodIsServed(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if h.Authorizer != nil {
		t.Fatalf("Authorizer = %v, want nil by default", h.Authorizer)
	}

	rec, resp := statelessRequest(t, h, "tools/call", "echo",
		statelessCall("tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"message": "hi"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, refused := resp["error"]; refused {
		t.Fatalf("an unconfigured handler refused a call: %v", resp)
	}
}

// TestAuthorizationUsesTheAuthenticatedSubject proves the decision is made on the
// context the request was served under, where a transport boundary put the
// principal it authenticated, and that the subject a request asserts in its own
// body never becomes that principal.
func TestAuthorizationUsesTheAuthenticatedSubject(t *testing.T) {
	var seen []MethodRequest
	var subjects []security.Subject
	var present []bool
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	h.Authorizer = MethodAuthorizerFunc(func(ctx context.Context, req MethodRequest) error {
		subject, ok := security.SubjectFromContext(ctx)
		seen = append(seen, req)
		subjects = append(subjects, subject)
		present = append(present, ok)
		return nil
	})

	claiming := statelessCall("tools/call", map[string]any{
		"name":      "echo",
		"subject":   "admin",
		"arguments": map[string]any{"message": "hi"},
	})

	authenticated := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := security.WithSubject(r.Context(), security.Subject{
			ID:    "alice",
			Kind:  security.SubjectUser,
			Roles: []string{"reader"},
		})
		h.ServeHTTP(w, r.WithContext(ctx))
	})
	if _, _ = statelessRequest(t, authenticated, "tools/call", "echo", claiming); len(seen) != 1 {
		t.Fatalf("authorizer calls = %d, want 1", len(seen))
	}
	if got := subjects[0]; got.ID != "alice" || !got.Authenticated() || !got.HasRole("reader") {
		t.Errorf("authorized subject = %+v, want the authenticated alice", got)
	}

	// Without a boundary that authenticated anyone, the claim in the body must
	// not fill the gap: no principal reaches the policy at all.
	seen, subjects, present = nil, nil, nil
	if _, _ = statelessRequest(t, h, "tools/call", "echo", claiming); len(seen) != 1 {
		t.Fatalf("authorizer calls = %d, want 1", len(seen))
	}
	if present[0] {
		t.Errorf("a subject reached the policy for an unauthenticated request: %+v", subjects[0])
	}
	if got := fmt.Sprintf("%+v", seen[0]); contains(got, "admin") {
		t.Errorf("the request's own subject claim reached the policy: %s", got)
	}
}


// TestRefusedNotificationIsForbidden proves a refusal is visible even when the
// request has no id to answer. Reporting 202 would tell the caller its message
// was accepted.
func TestRefusedNotificationIsForbidden(t *testing.T) {
	var seen []MethodRequest
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	h.Authorizer = recordAuthorizations(&seen, interaction.ErrUnauthorized)

	rec, _ := statelessRequest(t, h, "notifications/cancelled", "",
		map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] != "unauthorized" {
		t.Errorf("error = %q, want unauthorized", body["error"])
	}
	if len(seen) != 1 {
		t.Errorf("authorizer calls = %d, want 1", len(seen))
	}
}

// TestStatelessAuthorizationSeesTheDeclaredClient proves the identity a
// stateless request declares reaches the policy, which is what a per-client rule
// needs now that there is no handshake to record it.
func TestStatelessAuthorizationSeesTheDeclaredClient(t *testing.T) {
	var seen []MethodRequest
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	h.Authorizer = recordAuthorizations(&seen, nil)

	_, _ = statelessRequest(t, h, "tools/list", "", statelessCall("tools/list", map[string]any{
		"_meta": map[string]any{
			metaClientInfo: map[string]any{"name": "curl-client", "version": "1.0.0"},
		},
	}))
	if len(seen) != 1 {
		t.Fatalf("authorizer calls = %d, want 1", len(seen))
	}
	if seen[0].ClientName != "curl-client" || seen[0].ClientVersion != "1.0.0" {
		t.Errorf("authorized client = %q/%q, want curl-client/1.0.0", seen[0].ClientName, seen[0].ClientVersion)
	}
}

// TestLegacyRequestsReachTheAuthorizer proves the seam is not a property of the
// new revision: the handshake protocol is authorized too, starting with the
// initialize that would otherwise mint a session for anyone who asks.
func TestLegacyRequestsReachTheAuthorizer(t *testing.T) {
	post := func(t *testing.T, h http.Handler, sessionID string, body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
		t.Helper()
		payload, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
		req.Header.Set(headerProtocolVersion, legacyProtocolVersion)
		if sessionID != "" {
			req.Header.Set(headerSessionID, sessionID)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var resp map[string]any
		if rec.Body.Len() > 0 {
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		}
		return rec, resp
	}
	initialize := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": legacyProtocolVersion},
	}

	t.Run("RefusedInitializeMintsNoSession", func(t *testing.T) {
		var seen []MethodRequest
		h := NewStreamableHandler(setupRuntime(t))
		defer h.Close() //nolint:errcheck
		h.Authorizer = recordAuthorizations(&seen, interaction.ErrUnauthorized)

		rec, resp := post(t, h, "", initialize)
		if code, _ := resp["error"].(map[string]any)["code"].(float64); code != -32001 {
			t.Fatalf("initialize answered %v, want -32001", resp)
		}
		if got := rec.Header().Get(headerSessionID); got != "" {
			t.Errorf("%s = %q, want no session for a refused initialize", headerSessionID, got)
		}
		if len(seen) != 1 || seen[0].Method != "initialize" || seen[0].ProtocolVersion != legacyProtocolVersion {
			t.Errorf("authorizer saw %+v, want one initialize on %s", seen, legacyProtocolVersion)
		}
	})

	t.Run("SessionMethodsCarryTheirSession", func(t *testing.T) {
		var seen []MethodRequest
		h := NewStreamableHandler(setupRuntime(t))
		defer h.Close() //nolint:errcheck
		h.Authorizer = recordAuthorizations(&seen, nil)

		rec, _ := post(t, h, "", initialize)
		sessionID := rec.Header().Get(headerSessionID)
		if sessionID == "" {
			t.Fatalf("initialize minted no session: %s", rec.Body.String())
		}
		_, _ = post(t, h, sessionID, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
		_, resp := post(t, h, sessionID, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
		if _, ok := resp["result"].(map[string]any); !ok {
			t.Fatalf("tools/list answered %v", resp)
		}

		methods := make([]string, 0, len(seen))
		for _, call := range seen {
			methods = append(methods, call.Method)
			if call.ProtocolVersion != legacyProtocolVersion {
				t.Errorf("%s: authorized revision = %q", call.Method, call.ProtocolVersion)
			}
		}
		want := []string{"initialize", "notifications/initialized", "tools/list"}
		if fmt.Sprint(methods) != fmt.Sprint(want) {
			t.Fatalf("authorized methods = %v, want %v", methods, want)
		}
		if seen[2].SessionID != sessionID {
			t.Errorf("authorized session = %q, want %q", seen[2].SessionID, sessionID)
		}
		if seen[0].SessionID != "" {
			t.Errorf("initialize carried session %q, want none: it is the call that mints one", seen[0].SessionID)
		}
	})
}

// TestMethodAuthorizerFuncNilRefuses proves a policy that was wired but never
// set is a misconfiguration rather than a grant: a typed nil in the interface
// field is not the nil the transport checks for.
func TestMethodAuthorizerFuncNilRefuses(t *testing.T) {
	var policy MethodAuthorizerFunc
	if err := policy.AuthorizeMethod(context.Background(), MethodRequest{Method: "ping"}); !errors.Is(err, interaction.ErrUnauthorized) {
		t.Fatalf("nil MethodAuthorizerFunc returned %v, want ErrUnauthorized", err)
	}

	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	h.Authorizer = policy

	_, resp := statelessRequest(t, h, "ping", "", statelessCall("ping", nil))
	if code, _ := resp["error"].(map[string]any)["code"].(float64); code != -32001 {
		t.Fatalf("ping answered %v, want -32001", resp)
	}
}
