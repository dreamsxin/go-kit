package mcp

import (
	"context"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// Request authorization.
//
// Who may call which method is a property of the deployment, not of the
// protocol: it depends on the identity provider, the tenancy model, and which
// tools the operator considers dangerous. This transport therefore ships the
// seam and no policy — a handler with no Authorizer serves every method it
// implements, exactly as it did before this seam existed, and an application
// that needs a rule implements MethodAuthorizer and states the rule there.
//
// The division of labour with the rest of v2:
//
//   - security.Middleware (or the deployment's own HTTP middleware)
//     authenticates the caller and puts the subject in the context;
//   - MethodAuthorizer, here, decides whether that subject may reach this
//     method and target, before any registry or tool is consulted;
//   - interaction.Authorizer decides whether a tool call may run given its
//     arguments, which is the decision that needs the runtime session.
//
// The principal is read from the context by the authorizer itself — with
// security.SubjectFromContext, or whatever the deployment's authentication layer
// stored. This package deliberately has no opinion about how an identity is
// represented, and never fills a principal in from the request body: a subject a
// request asserts about itself is a claim, not an authentication.

// MethodRequest describes one MCP request to a MethodAuthorizer. It is what the
// transport knows about the request before it dispatches anything.
type MethodRequest struct {
	// Method is the JSON-RPC method, for example "tools/call". It is the name
	// the server would dispatch, so a policy keyed on it cannot be bypassed by
	// a header that says something else.
	Method string

	// Target names the tool, prompt, or resource the method addresses, and is
	// empty for the methods that address the server itself.
	Target string

	// ProtocolVersion is the revision the request selected.
	ProtocolVersion string

	// ClientName and ClientVersion are what a 2026-07-28 request declared in
	// _meta. Both are empty on 2025-06-18, which carries client identity in the
	// handshake rather than in the request, and both are self-reported: they
	// identify software, not a principal.
	ClientName    string
	ClientVersion string

	// SessionID is the 2025-06-18 transport session the request presented, and
	// is empty on 2026-07-28, which has no sessions.
	SessionID string

	// Header is the request's HTTP header, where credentials, tenant hints and
	// tracing identifiers live. Treat it as read-only.
	Header http.Header
}

// MethodAuthorizer decides whether one MCP request may proceed. A nil error
// allows it; any other error refuses it.
//
// The context is the one the request is served under, so an implementation reads
// the authenticated principal from it — security.SubjectFromContext for a
// deployment using this framework's security package.
//
// Refusals should wrap interaction.ErrUnauthorized when the reason is worth
// naming to the caller — an unwrapped error is still a refusal, and its message
// still reaches the caller as the error's data.
type MethodAuthorizer interface {
	AuthorizeMethod(ctx context.Context, req MethodRequest) error
}

// MethodAuthorizerFunc adapts a function into a MethodAuthorizer.
type MethodAuthorizerFunc func(context.Context, MethodRequest) error

// AuthorizeMethod calls f. A nil f refuses: it reaches a MethodAuthorizer field
// only as a typed nil, which the transport's nil check cannot see, and a policy
// that was never wired is a misconfiguration — not a grant.
func (f MethodAuthorizerFunc) AuthorizeMethod(ctx context.Context, req MethodRequest) error {
	if f == nil {
		return interaction.ErrUnauthorized
	}
	return f(ctx, req)
}

// authorizeMethod asks the deployment's policy about one request. It reports
// false once it has answered the request itself.
//
// Stable: mcp.method-authorization — every request reaches the configured MethodAuthorizer, with its method and target, before anything is dispatched.
// Covered by: TestMethodAuthorizerSeesEveryRequest, TestLegacyRequestsReachTheAuthorizer
//
// Stable: mcp.no-authorization-policy — a transport with no MethodAuthorizer applies no policy of its own.
// Covered by: TestWithoutAnAuthorizerEveryMethodIsServed
//
// Stable: mcp.authorization-subject — authorization sees the context the request was served under, and no principal is ever taken from the request body.
// Covered by: TestAuthorizationUsesTheAuthenticatedSubject
func (h *StreamableHandler) authorizeMethod(ctx context.Context, w http.ResponseWriter, r *http.Request, req request, version string) bool {
	if h.Authorizer == nil {
		return true
	}
	// An empty method is this client answering a request the server made, not
	// one it is asking for. There is nothing to authorize and no method name a
	// policy could decide on.
	if req.Method == "" {
		return true
	}

	call := MethodRequest{
		Method:          req.Method,
		Target:          routingTarget(req.Method, req.Params),
		ProtocolVersion: version,
		SessionID:       r.Header.Get(headerSessionID),
		Header:          r.Header,
	}
	if version == protocolVersion {
		call.SessionID = ""
		call.ClientName, call.ClientVersion, _ = IdentityFromContext(ctx)
	}

	err := h.Authorizer.AuthorizeMethod(ctx, call)
	if err == nil {
		return true
	}
	if req.ID == nil {
		// A notification has no id to answer, so the refusal has to be the HTTP
		// status. Reporting 202 would tell the caller its message was accepted.
		writeHTTPError(w, http.StatusForbidden, "unauthorized", err.Error())
		return false
	}
	writeResponse(w, response{JSONRPC: jsonRPCVersion, ID: req.ID, Error: authorizationError(err)})
	return false
}


// authorizationError renders a refusal. A refusal that names no known
// interaction failure is unauthorized rather than an internal error: the
// authorizer was asked whether to proceed and did not say yes, and reporting
// that as a server fault would invite a client to retry it.
func authorizationError(err error) *rpcError {
	code := RPCCodeForInteractionError(err)
	if code == -32603 {
		code = -32001
	}
	return newError(code, messageForCode(code), err.Error())
}
