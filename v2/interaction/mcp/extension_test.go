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
)

// echoExtension is a deployment-defined extension: two methods under one
// reverse-DNS id, which is all the framework knows about it.
func echoExtension(t *testing.T, seen *map[string]any) Extension {
	t.Helper()
	return Extension{
		Name:    "vendor.example/echo",
		Version: "1.2.0",
		Config:  map[string]any{"maxLength": 64},
		Methods: map[string]ExtensionMethod{
			"vendor.example/echo/say": func(ctx context.Context, params json.RawMessage) (any, error) {
				if seen != nil {
					*seen, _ = MetaFromContext(ctx)
				}
				var body struct {
					Text string `json:"text"`
				}
				if len(params) > 0 {
					_ = json.Unmarshal(params, &body)
				}
				return map[string]any{"said": body.Text}, nil
			},
			"vendor.example/echo/fail": func(context.Context, json.RawMessage) (any, error) {
				return nil, fmt.Errorf("%w: text is required", interaction.ErrInvalidArgument)
			},
		},
	}
}

// TestRegisterExtensionRefusesAnUnusableDeclaration proves the declaration is
// checked where it is made. An id or method name that only fails later fails on
// the wire, in front of a client that trusted the capability document.
func TestRegisterExtensionRefusesAnUnusableDeclaration(t *testing.T) {
	cases := []struct {
		name string
		ext  Extension
	}{
		{name: "NoName", ext: Extension{Version: "1"}},
		{name: "NoVersion", ext: Extension{Name: "vendor.example/x"}},
		{name: "NotNamespaced", ext: Extension{Name: "echo", Version: "1"}},
		{name: "VendorIsNotReverseDNS", ext: Extension{Name: "vendor/echo", Version: "1"}},
		{name: "NoOwnName", ext: Extension{Name: "vendor.example/", Version: "1"}},
		{name: "ReservedNamespace", ext: Extension{Name: "io.modelcontextprotocol/tasks", Version: "1"}},
		{name: "NameWithWhitespace", ext: Extension{Name: "vendor.example/ec ho", Version: "1"}},
		{
			name: "MethodOutsideItsNamespace",
			ext: Extension{Name: "vendor.example/echo", Version: "1", Methods: map[string]ExtensionMethod{
				"other.example/echo/say": func(context.Context, json.RawMessage) (any, error) { return nil, nil },
			}},
		},
		{
			name: "MethodWithoutAnImplementation",
			ext: Extension{Name: "vendor.example/echo", Version: "1", Methods: map[string]ExtensionMethod{
				"vendor.example/echo/say": nil,
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewStreamableHandler(nil)
			defer h.Close() //nolint:errcheck
			if err := h.RegisterExtension(tc.ext); err == nil {
				t.Fatalf("RegisterExtension(%+v) = nil, want an error", tc.ext)
			}
		})
	}

	t.Run("CoreMethodTakeover", func(t *testing.T) {
		h := NewStreamableHandler(setupRuntime(t))
		defer h.Close() //nolint:errcheck
		// The namespace check has to come second here: this method name does
		// carry its extension's prefix, and is still a method the protocol owns.
		err := h.RegisterExtension(Extension{Name: "tools.example/x", Version: "1", Methods: map[string]ExtensionMethod{
			"tools.example/x/tools/call": func(context.Context, json.RawMessage) (any, error) { return nil, nil },
		}})
		if err != nil {
			t.Fatalf("a namespaced method that merely contains a core name: %v", err)
		}
		if err := h.RegisterExtension(Extension{Name: "core.example/x", Version: "1", Methods: map[string]ExtensionMethod{
			"ping": func(context.Context, json.RawMessage) (any, error) { return nil, nil },
		}}); err == nil {
			t.Fatal("an extension claimed ping")
		}
	})

	t.Run("RegisteredTwice", func(t *testing.T) {
		h := NewStreamableHandler(setupRuntime(t))
		defer h.Close() //nolint:errcheck
		if err := h.RegisterExtension(echoExtension(t, nil)); err != nil {
			t.Fatalf("first RegisterExtension: %v", err)
		}
		if err := h.RegisterExtension(echoExtension(t, nil)); err == nil {
			t.Fatal("the same extension registered twice was accepted")
		}
	})
}

// TestExtensionsAreDeclaredOnlyWhenRegistered proves extensions are off until a
// deployment enables one, and that an enabled one is described well enough for a
// client to decide whether it understands this version of it.
func TestExtensionsAreDeclaredOnlyWhenRegistered(t *testing.T) {
	bare := NewStreamableHandler(setupRuntime(t))
	defer bare.Close() //nolint:errcheck

	_, resp := statelessRequest(t, bare, "server/discover", "", statelessCall("server/discover", nil))
	caps, _ := resp["result"].(map[string]any)["capabilities"].(map[string]any)
	if _, declared := caps["extensions"]; declared {
		t.Fatalf("a server with nothing registered declares extensions: %v", caps)
	}

	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(echoExtension(t, nil)); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	_, resp = statelessRequest(t, h, "server/discover", "", statelessCall("server/discover", nil))
	caps, _ = resp["result"].(map[string]any)["capabilities"].(map[string]any)
	declared, ok := caps["extensions"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities declare no extensions: %v", caps)
	}
	entry, ok := declared["vendor.example/echo"].(map[string]any)
	if !ok {
		t.Fatalf("extensions = %v, want the registered id", declared)
	}
	if entry["version"] != "1.2.0" {
		t.Errorf("version = %v, want 1.2.0", entry["version"])
	}
	config, _ := entry["config"].(map[string]any)
	if config["maxLength"] != float64(64) {
		t.Errorf("config = %v, want the extension's own object", entry["config"])
	}
	methods, _ := entry["methods"].([]any)
	want := []any{"vendor.example/echo/fail", "vendor.example/echo/say"}
	if fmt.Sprint(methods) != fmt.Sprint(want) {
		t.Errorf("methods = %v, want %v in a stable order", methods, want)
	}

	// The handshake revision declares the same capabilities, so a client on it
	// is not told a different server exists.
	rec, _ := legacyRequest(t, h, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": legacyProtocolVersion},
	})
	var initialize struct {
		Result struct {
			Capabilities struct {
				Extensions map[string]any `json:"extensions"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &initialize); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if _, ok := initialize.Result.Capabilities.Extensions["vendor.example/echo"]; !ok {
		t.Errorf("initialize capabilities declare no extensions: %s", rec.Body.String())
	}
}

// legacyRequest posts one 2025-06-18 request, with a session header when given.
func legacyRequest(t *testing.T, h http.Handler, sessionID string, body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
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

// TestExtensionMethodIsDispatchedOnBothRevisions proves an extension is served
// wherever the core protocol is, and that its failures are classified rather
// than flattened into one code.
func TestExtensionMethodIsDispatchedOnBothRevisions(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(echoExtension(t, nil)); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	_, resp := statelessRequest(t, h, "vendor.example/echo/say", "",
		statelessCall("vendor.example/echo/say", map[string]any{"text": "hi"}))
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("stateless extension call answered %v", resp)
	}
	if result["said"] != "hi" {
		t.Errorf("said = %v, want hi", result["said"])
	}

	_, resp = statelessRequest(t, h, "vendor.example/echo/fail", "",
		statelessCall("vendor.example/echo/fail", nil))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("a failing extension method answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32602 {
		t.Errorf("error code = %v, want -32602 for an invalid-argument failure", rpcErr["code"])
	}

	rec, _ := legacyRequest(t, h, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": legacyProtocolVersion},
	})
	sessionID := rec.Header().Get(headerSessionID)
	if sessionID == "" {
		t.Fatalf("initialize minted no session: %s", rec.Body.String())
	}
	_, _ = legacyRequest(t, h, sessionID, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	_, resp = legacyRequest(t, h, sessionID, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "vendor.example/echo/say",
		"params": map[string]any{"text": "session"},
	})
	result, ok = resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("handshake extension call answered %v", resp)
	}
	if result["said"] != "session" {
		t.Errorf("said = %v, want session", result["said"])
	}
}

// TestUnregisteredExtensionMethodIsNotFound proves the fallback rule: a server
// that did not enable an extension looks exactly like a server that never had
// one, so a client can detect that and use core protocol instead.
func TestUnregisteredExtensionMethodIsNotFound(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck

	_, resp := statelessRequest(t, h, "vendor.example/echo/say", "",
		statelessCall("vendor.example/echo/say", map[string]any{"text": "hi"}))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("an unregistered extension method answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32601 {
		t.Errorf("error code = %v, want -32601", rpcErr["code"])
	}
}

// TestExtensionMethodIsAuthorized proves an extension does not route around the
// policy: it is a method, and the authorizer sees it like any other.
func TestExtensionMethodIsAuthorized(t *testing.T) {
	invoked := false
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(Extension{
		Name: "vendor.example/echo", Version: "1",
		Methods: map[string]ExtensionMethod{
			"vendor.example/echo/say": func(context.Context, json.RawMessage) (any, error) {
				invoked = true
				return map[string]any{}, nil
			},
		},
	}); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}
	var seen []MethodRequest
	h.Authorizer = recordAuthorizations(&seen, interaction.ErrUnauthorized)

	_, resp := statelessRequest(t, h, "vendor.example/echo/say", "",
		statelessCall("vendor.example/echo/say", nil))
	if code, _ := resp["error"].(map[string]any)["code"].(float64); code != -32001 {
		t.Fatalf("a refused extension call answered %v, want -32001", resp)
	}
	if invoked {
		t.Error("the extension method ran despite the refusal")
	}
	if len(seen) != 1 || seen[0].Method != "vendor.example/echo/say" {
		t.Errorf("authorizer saw %+v, want the extension method", seen)
	}
}

// TestClientExtensionIsReadablePerRequest proves the other half of the fallback
// rule: an implementation can tell whether this caller supports an extension,
// and so can choose core behaviour instead of assuming.
func TestClientExtensionIsReadablePerRequest(t *testing.T) {
	type observation struct {
		config map[string]any
		ok     bool
	}
	var seen observation
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(Extension{
		Name: "vendor.example/echo", Version: "1",
		Methods: map[string]ExtensionMethod{
			"vendor.example/echo/say": func(ctx context.Context, _ json.RawMessage) (any, error) {
				seen.config, seen.ok = ClientExtensionFromContext(ctx, "vendor.example/echo")
				return map[string]any{}, nil
			},
		},
	}); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	_, _ = statelessRequest(t, h, "vendor.example/echo/say", "", statelessCall("vendor.example/echo/say", map[string]any{
		"_meta": map[string]any{
			metaClientCapabilities: map[string]any{
				"extensions": map[string]any{
					"vendor.example/echo": map[string]any{"version": "1", "maxLength": 8},
				},
			},
		},
	}))
	if !seen.ok {
		t.Fatal("a declared client extension was not visible to the method")
	}
	if seen.config["maxLength"] != float64(8) {
		t.Errorf("client extension config = %v, want the declared object", seen.config)
	}

	seen = observation{}
	_, _ = statelessRequest(t, h, "vendor.example/echo/say", "", statelessCall("vendor.example/echo/say", nil))
	if seen.ok {
		t.Errorf("a client that declared nothing was reported as supporting the extension: %v", seen.config)
	}
}

// TestUnknownMetaKeyIsCarriedNotRefused proves this server does not stand between
// a client extension and the code that implements it: a key it has never heard
// of is neither an error nor dropped.
func TestUnknownMetaKeyIsCarriedNotRefused(t *testing.T) {
	var seen map[string]any
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(echoExtension(t, &seen)); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	rec, resp := statelessRequest(t, h, "vendor.example/echo/say", "", statelessCall("vendor.example/echo/say", map[string]any{
		"text": "hi",
		"_meta": map[string]any{
			metaClientInfo:              map[string]any{"name": "curl-client", "version": "1.0.0"},
			"vendor.example/trace":      map[string]any{"id": "abc"},
			"some.other.vendor/unknown": "whatever",
		},
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, refused := resp["error"]; refused {
		t.Fatalf("an unknown _meta key was refused: %v", resp)
	}
	trace, _ := seen["vendor.example/trace"].(map[string]any)
	if trace["id"] != "abc" {
		t.Errorf("_meta = %v, want the unrecognised extension key carried through", seen)
	}
	if seen["some.other.vendor/unknown"] != "whatever" {
		t.Errorf("_meta = %v, want every key the client sent", seen)
	}
	if _, ok := seen[metaClientInfo]; !ok {
		t.Errorf("_meta = %v, want the keys this server does understand too", seen)
	}

	// A `_meta` block that is not an object at all is still a malformed request.
	_, resp = statelessRequest(t, h, "vendor.example/echo/say", "",
		statelessCall("vendor.example/echo/say", map[string]any{"_meta": "not-an-object"}))
	if _, refused := resp["error"]; !refused {
		t.Errorf("a non-object _meta was accepted: %v", resp)
	}
}

// TestMetaFromContextReportsNothingOnTheHandshakeRevision proves the accessor
// does not invent a stateless request where there was none.
func TestMetaFromContextReportsNothingOnTheHandshakeRevision(t *testing.T) {
	if meta, ok := MetaFromContext(context.Background()); ok || meta != nil {
		t.Fatalf("MetaFromContext(background) = %v, %v; want nil, false", meta, ok)
	}
	if config, ok := ClientExtensionFromContext(context.Background(), "vendor.example/echo"); ok || config != nil {
		t.Fatalf("ClientExtensionFromContext(background) = %v, %v; want nil, false", config, ok)
	}
}

// TestExtensionMethodErrorWithoutASentinelIsInternal proves an extension that
// breaks is reported as this server's fault, not as the caller's bad request.
func TestExtensionMethodErrorWithoutASentinelIsInternal(t *testing.T) {
	h := NewStreamableHandler(setupRuntime(t))
	defer h.Close() //nolint:errcheck
	if err := h.RegisterExtension(Extension{
		Name: "vendor.example/echo", Version: "1",
		Methods: map[string]ExtensionMethod{
			"vendor.example/echo/say": func(context.Context, json.RawMessage) (any, error) {
				return nil, errors.New("the extension's backing store is down")
			},
		},
	}); err != nil {
		t.Fatalf("RegisterExtension: %v", err)
	}

	_, resp := statelessRequest(t, h, "vendor.example/echo/say", "", statelessCall("vendor.example/echo/say", nil))
	if code, _ := resp["error"].(map[string]any)["code"].(float64); code != -32603 {
		t.Fatalf("answered %v, want -32603", resp)
	}
}
