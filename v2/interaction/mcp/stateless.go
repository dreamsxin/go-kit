package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// The stateless request model of MCP 2026-07-28.
//
// 2025-06-18 opened with an initialize/initialized handshake and carried an
// Mcp-Session-Id header on every later request, which pinned a client to one
// server instance and to shared session storage. 2026-07-28 retires both: a
// request describes itself in _meta, capabilities are fetched on demand with
// server/discover, and the method and target name travel in headers so a gateway
// can route and meter without parsing the body. Both revisions are served here,
// chosen by the MCP-Protocol-Version header, because the old one keeps its
// twelve-month deprecation window.
const (
	// headerMethod and headerName let infrastructure route and authorize on
	// headers instead of the JSON-RPC body.
	headerMethod = "Mcp-Method"
	headerName   = "Mcp-Name"

	// Per-request identity keys, reverse-DNS namespaced by the specification.
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"

	defaultListCacheTTL   = time.Minute
	defaultListCacheScope = "private"
)

// statelessTargetedMethods are the methods that address one named tool, prompt
// or resource, and so must repeat that name in the Mcp-Name header. Every other
// method addresses the server, and must not send one — including a method this
// server does not know, whose name a gateway could otherwise route on.
var statelessTargetedMethods = map[string]bool{
	"tools/call":     true,
	"prompts/get":    true,
	"resources/read": true,
}

// statelessRetiredMethods no longer exist in 2026-07-28. Naming them in the
// error is worth more than a bare "method not found": a client sending them is a
// client that believes it opened a session.
var statelessRetiredMethods = map[string]string{
	"initialize":                "initialize was retired in " + protocolVersion + "; call server/discover instead, or send MCP-Protocol-Version: " + legacyProtocolVersion,
	"notifications/initialized": "notifications/initialized was retired in " + protocolVersion + "; requests need no handshake",
}

// requestIdentity is what a 2026-07-28 request says about its caller. It
// replaces the session record the handshake used to build.
type requestIdentity struct {
	protocol     string
	info         map[string]any
	capabilities map[string]any
}

type statelessContextKey struct{}

// IdentityFromContext reports the client identity a stateless request carried in
// _meta, and whether the request was a stateless one at all. Tools use it where
// they would have read session state under the handshake protocol.
func IdentityFromContext(ctx context.Context) (name, version string, ok bool) {
	identity, ok := ctx.Value(statelessContextKey{}).(requestIdentity)
	if !ok {
		return "", "", false
	}
	name, _ = identity.info["name"].(string)
	version, _ = identity.info["version"].(string)
	return name, version, true
}

// ClientCapabilityFromContext reports whether a stateless request declared the
// named client capability.
func ClientCapabilityFromContext(ctx context.Context, capability string) bool {
	identity, ok := ctx.Value(statelessContextKey{}).(requestIdentity)
	if !ok {
		return false
	}
	_, declared := identity.capabilities[capability]
	return declared
}

// negotiateProtocolVersion resolves the revision a request speaks.
//
// An absent header means the pre-2026 client that never sent one, so it selects
// the handshake protocol rather than failing: the header only became mandatory
// in the revision that removed the handshake.
//
// Stable: mcp.version-negotiation — MCP-Protocol-Version selects the request model, and an absent header selects 2025-06-18.
// Covered by: TestStatelessRequestNeedsNoSession, TestStreamableInitialize
func negotiateProtocolVersion(r *http.Request) (string, error) {
	switch version := strings.TrimSpace(r.Header.Get(headerProtocolVersion)); version {
	case "":
		return legacyProtocolVersion, nil
	case protocolVersion:
		return protocolVersion, nil
	case legacyProtocolVersion:
		return legacyProtocolVersion, nil
	default:
		return "", fmt.Errorf("unsupported MCP protocol version %q; server supports %q and %q",
			version, protocolVersion, legacyProtocolVersion)
	}
}

// readRequestIdentity reads the _meta block a stateless request carries.
func readRequestIdentity(params json.RawMessage) (requestIdentity, error) {
	var envelope struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &envelope); err != nil {
			return requestIdentity{}, fmt.Errorf("params are not an object: %w", err)
		}
	}
	identity := requestIdentity{protocol: protocolVersion}
	if raw, ok := envelope.Meta[metaProtocolVersion]; ok {
		var declared string
		if err := json.Unmarshal(raw, &declared); err != nil {
			return requestIdentity{}, fmt.Errorf("%s must be a string", metaProtocolVersion)
		}
		if declared != protocolVersion {
			return requestIdentity{}, fmt.Errorf("%s is %q but the request header says %q",
				metaProtocolVersion, declared, protocolVersion)
		}
	}
	if raw, ok := envelope.Meta[metaClientInfo]; ok {
		if err := json.Unmarshal(raw, &identity.info); err != nil {
			return requestIdentity{}, fmt.Errorf("%s must be an object", metaClientInfo)
		}
	}
	if raw, ok := envelope.Meta[metaClientCapabilities]; ok {
		if err := json.Unmarshal(raw, &identity.capabilities); err != nil {
			return requestIdentity{}, fmt.Errorf("%s must be an object", metaClientCapabilities)
		}
	}
	return identity, nil
}

// routingTarget reads the name a method addresses, which the Mcp-Name header
// must repeat.
func routingTarget(method string, params json.RawMessage) string {
	var target struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &target)
	}
	switch method {
	case "tools/call", "prompts/get":
		return target.Name
	case "resources/read":
		return target.URI
	default:
		return ""
	}
}

// validateRoutingHeaders checks that the headers a gateway routes on describe
// the body it forwarded. A mismatch is a request that would be metered, routed
// and authorized as something other than what it does.
func validateRoutingHeaders(r *http.Request, req request) error {
	method := strings.TrimSpace(r.Header.Get(headerMethod))
	if method == "" {
		return fmt.Errorf("%s header is required", headerMethod)
	}
	if method != req.Method {
		return fmt.Errorf("%s header is %q but the request method is %q", headerMethod, method, req.Method)
	}

	name := strings.TrimSpace(r.Header.Get(headerName))
	if !statelessTargetedMethods[req.Method] {
		if name != "" {
			return fmt.Errorf("%s header is %q but %s addresses no target", headerName, name, req.Method)
		}
		return nil
	}
	target := routingTarget(req.Method, req.Params)
	if name == "" {
		return fmt.Errorf("%s header is required for %s", headerName, req.Method)
	}
	if target != "" && name != target {
		return fmt.Errorf("%s header is %q but %s addresses %q", headerName, name, req.Method, target)
	}
	return nil
}

// ─── stateless POST ──────────────────────────────────────────────────────────

// handleStatelessPost answers one self-describing request. Nothing it does
// outlives the response: no session is minted, looked up, or required, which is
// what lets a load balancer place any request on any instance.
//
// Stable: mcp.stateless-request — a 2026-07-28 request carries its own identity and needs no session of any kind.
// Covered by: TestStatelessRequestNeedsNoSession, TestStatelessRetiresTheHandshake
func (h *StreamableHandler) handleStatelessPost(w http.ResponseWriter, r *http.Request) {
	rawBody, ok := h.readPostBody(w, r)
	if !ok {
		return
	}

	var req request
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}
	if req.JSONRPC != jsonRPCVersion {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", "jsonrpc must be 2.0")})
		return
	}
	if err := validateRoutingHeaders(r, req); err != nil {
		writeHTTPError(w, http.StatusBadRequest, "invalid_routing_header", err.Error())
		return
	}
	if reason, retired := statelessRetiredMethods[req.Method]; retired {
		if req.ID == nil {
			// A retired notification is not worth an error nobody reads.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32601, "method not found", reason)})
		return
	}

	identity, err := readRequestIdentity(req.Params)
	if err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: newError(-32600, "invalid request", err.Error())})
		return
	}
	ctx := context.WithValue(r.Context(), statelessContextKey{}, identity)

	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	if req.Method == "server/discover" {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: h.buildDiscoverResult()})
		return
	}

	resp := h.core.dispatch(ctx, req)
	h.applyCacheHints(req.Method, resp)
	markResultType(req.Method, resp)
	writeResponse(w, resp)
}

// buildDiscoverResult answers server/discover, the one call that replaces the
// handshake for a client that wants capabilities before it commits to anything.
//
// Stable: mcp.server-discover — server/discover reports the supported versions, the capabilities, and how long both may be cached.
// Covered by: TestStatelessServerDiscover
func (h *StreamableHandler) buildDiscoverResult() map[string]any {
	capabilities := h.core.buildCapabilities()
	// logging/setLevel sets a threshold on a session, and this revision has no
	// session to hold one. Advertising it would promise a call that cannot work.
	delete(capabilities, "logging")
	result := map[string]any{
		"resultType":        "complete",
		"supportedVersions": []string{protocolVersion, legacyProtocolVersion},
		"capabilities":      capabilities,
		"instructions":      serverInstructions,
		"_meta": map[string]any{
			metaServerInfo: map[string]any{
				"name":    serverName,
				"title":   serverTitle,
				"version": serverVersion,
			},
		},
	}
	h.addCacheHints(result)
	return result
}

// applyCacheHints adds the caching contract to the results that carry one.
//
// Stable: mcp.list-cache-hints — a list or read result on 2026-07-28 carries ttlMs and cacheScope.
// Covered by: TestStatelessListResultsCarryCacheHints
func (h *StreamableHandler) applyCacheHints(method string, resp response) {
	switch method {
	case "tools/list", "prompts/list", "resources/list", "resources/templates/list", "resources/read":
	default:
		return
	}
	if result, ok := resp.Result.(map[string]any); ok {
		h.addCacheHints(result)
	}
}

func (h *StreamableHandler) addCacheHints(result map[string]any) {
	ttl := h.ListCacheTTL
	if ttl <= 0 {
		ttl = defaultListCacheTTL
	}
	scope := strings.TrimSpace(h.ListCacheScope)
	if scope == "" {
		scope = defaultListCacheScope
	}
	result["ttlMs"] = ttl.Milliseconds()
	result["cacheScope"] = scope
}

// ─── multi round-trip requests ───────────────────────────────────────────────
//
// A stateless server cannot hold a call open to ask the caller a question, so a
// tool that needs one returns instead: resultType "input_required" with the
// questions and an opaque requestState. The caller answers, repeats the call
// with inputResponses and the same state, and the tool runs again — this time
// with the answers in its context.

var (
	// errInputRequiredNeedsStateless reports a tool asking for input on the
	// handshake revision, where the result type does not exist.
	errInputRequiredNeedsStateless = errors.New("an input_required result belongs to " + protocolVersion)
)

// missingCapabilityError reports a question the caller never said it could
// answer. Asking anyway would hang a client that has no code for it.
type missingCapabilityError struct {
	kinds []string
	cause error
}

func (e *missingCapabilityError) Error() string {
	return fmt.Sprintf("the client declared no %s capability", strings.Join(e.kinds, ", "))
}

func (e *missingCapabilityError) Unwrap() error { return e.cause }

// requestIdentityFromContext reports the identity a stateless request carried,
// and whether the request was a stateless one.
func requestIdentityFromContext(ctx context.Context) (requestIdentity, bool) {
	identity, ok := ctx.Value(statelessContextKey{}).(requestIdentity)
	return identity, ok
}

// inputRequiredResult turns a tool's request for input into the interim result
// the caller answers and retries.
//
// Stable: mcp.input-required — a tool that needs input answers resultType "input_required" with inputRequests and an opaque requestState.
// Covered by: TestStatelessToolAsksForInputAndResumes
//
// Stable: mcp.missing-client-capability — a question the caller never declared it can answer is -32021 naming the capability.
// Covered by: TestStatelessInputRequiresADeclaredCapability
func inputRequiredResult(ctx context.Context, needsInput *interaction.InputRequired) (map[string]any, error) {
	identity, stateless := requestIdentityFromContext(ctx)
	if !stateless {
		return nil, fmt.Errorf("%w, and this request selected %s: %w",
			errInputRequiredNeedsStateless, legacyProtocolVersion, needsInput)
	}

	var missing []string
	for _, kind := range needsInput.Kinds() {
		if _, declared := identity.capabilities[kind]; !declared {
			missing = append(missing, kind)
		}
	}
	if len(missing) > 0 {
		return nil, &missingCapabilityError{kinds: missing, cause: needsInput}
	}

	state, err := encodeRequestState(needsInput.State)
	if err != nil {
		return nil, err
	}
	requests := make(map[string]any, len(needsInput.Requests))
	for id, request := range needsInput.Requests {
		entry := map[string]any{"kind": request.Kind}
		if len(request.Params) > 0 {
			entry["params"] = request.Params
		}
		requests[id] = entry
	}
	result := map[string]any{
		"resultType":    "input_required",
		"inputRequests": requests,
	}
	if state != "" {
		result["requestState"] = state
	}
	return result, nil
}

// toolCallError classifies what stopped a tool call. An unanswerable question is
// not the caller's bad argument, and neither is a tool asking for input on a
// revision that cannot carry the request.
func toolCallError(err error) *rpcError {
	var missing *missingCapabilityError
	switch {
	case errors.As(err, &missing):
		return &rpcError{
			Code:    -32021,
			Message: "missing required client capability",
			Data:    map[string]any{"requiredCapabilities": missing.kinds, "reason": missing.Error()},
		}
	case errors.Is(err, errInputRequiredNeedsStateless):
		return newError(-32603, "internal error", err.Error())
	default:
		return newError(-32602, "invalid argument", err.Error())
	}
}

// encodeRequestState renders the state a tool wants echoed back. It is base64 so
// a caller has no reason to read it, and JSON so this server can.
func encodeRequestState(state map[string]any) (string, error) {
	if len(state) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode request state: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

// decodeRequestState reads back what a tool asked to have echoed. It arrives
// from the caller, so it is client input: a tool validates it like any other.
func decodeRequestState(encoded string) (map[string]any, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("requestState is not base64: %w", err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("requestState is not a JSON object: %w", err)
	}
	return state, nil
}

// markResultType states that a result is final, so a caller can tell a finished
// call from one waiting on an answer without inspecting the payload.
//
// Stable: mcp.result-type — a tools/call, resources/read or prompts/get result says whether it is "complete".
// Covered by: TestStatelessToolAsksForInputAndResumes
func markResultType(method string, resp response) {
	switch method {
	case "tools/call", "resources/read", "prompts/get":
	default:
		return
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return
	}
	if _, stated := result["resultType"]; !stated {
		result["resultType"] = "complete"
	}
}
