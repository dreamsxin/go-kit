// Package mcp exposes a Model Context Protocol (MCP) compliant JSON-RPC 2.0
// HTTP endpoint for go-kit services. It bridges the interaction.Runtime
// (tools, resources, prompts) with any MCP-capable AI client.
//
// # Protocol Conformance
//
// The server speaks two revisions of the specification, chosen per request by
// the MCP-Protocol-Version header:
//
//   - 2026-07-28, the stateless revision. A request carries its own protocol
//     version, client identity and client capabilities in params._meta, names
//     its method and target in the Mcp-Method and Mcp-Name headers, and needs no
//     session of any kind: any request may land on any instance. server/discover
//     reports capabilities for a client that wants them before it commits.
//     tools/list, prompts/list, resources/list, resources/templates/list and
//     resources/read carry ttlMs and cacheScope so a catalog can be cached. A
//     tool that needs input mid-call answers resultType "input_required" with
//     inputRequests and an opaque requestState, and the client repeats the call
//     with inputResponses; see interaction.InputRequired. Set RequestStateKey to
//     authenticate that state, and the same key on every instance.
//   - 2025-06-18, the handshake revision, kept for its deprecation window. An
//     absent MCP-Protocol-Version header selects it, because it predates the
//     header being mandatory. initialize mints an Mcp-Session-Id that every
//     later request repeats.
//
// Capabilities are declared dynamically from the providers attached to the
// runtime, on both revisions.
//
// # Transport
//
// StreamableHandler implements the full MCP Streamable HTTP transport.
//
// On 2026-07-28, POST is the whole transport: there is no session to stream
// against or to delete, so GET and DELETE are answered 405.
//
// On 2025-06-18 it serves:
//   - POST for client JSON-RPC messages (requests, notifications, responses)
//   - GET  for persistent SSE streams (server-initiated messages)
//   - DELETE for explicit session termination
//   - Session management via Mcp-Session-Id header
//   - SSE streaming responses when client sends Accept: text/event-stream
//   - Server-initiated requests (sampling/createMessage)
//   - Protocol-version, initialization-state, and browser Origin validation
//
// NewHandler is a convenience alias for NewStreamableHandler.
//
// # Supported Methods
//
// Base protocol: ping, and server/discover on 2026-07-28. initialize and
// notifications/initialized belong to 2025-06-18; on the stateless revision they
// are answered -32601 naming server/discover.
//
// Tools: tools/list, tools/call — discover and invoke service methods.
//
// Resources: resources/list, resources/read, resources/templates/list —
// read server-exposed data artifacts. Only advertised when a ResourceProvider
// is attached via Runtime.WithResources.
//
// Prompts: prompts/list, prompts/get — list and render reusable prompt
// templates. Only advertised when a PromptProvider is attached via
// Runtime.WithPrompts.
//
// Completions: completion/complete — provide argument auto-completion for
// prompts. Advertised when the PromptProvider implements PromptCompleter.
//
// Logging: logging/setLevel — adjust server log verbosity.
//
// # Request Authorization
//
// Who may call which method is a property of the deployment, so the transport
// ships the seam and no policy: a handler with no Authorizer serves every method
// it implements. Set StreamableHandler.Authorizer to a MethodAuthorizer and every
// request — on both revisions, notifications included — reaches it with its
// method, its target name, the HTTP header, and the context the request is served
// under, before any registry, provider or tool is consulted. A nil error allows
// the request; any other error refuses it, as -32001 for a refusal that names no
// other failure, or 403 when the request has no id to answer.
//
// The three seams do different jobs:
//
//   - security.Middleware, or the deployment's own HTTP middleware,
//     authenticates the caller and stores the subject with security.WithSubject.
//   - MethodAuthorizer decides whether that subject may reach this method and
//     target. It reads the principal from the context — this package has no
//     opinion about how an identity is represented — and never takes one from
//     the request body, where a subject is a claim rather than an authentication.
//   - interaction.Authorizer decides whether a tool call may run given its
//     arguments, which is the decision that needs the runtime session.
//
// # Notifications
//
// The handler can send server-initiated notifications to the client
// via SSE streams. Available notification methods:
//
//   - LogNotification: sends notifications/message for server-side logging
//   - ProgressNotification: sends notifications/progress for long operations
//   - ResourceUpdatedNotification: sends notifications/resources/updated
//   - ResourceListChangedNotification: sends notifications/resources/list_changed
//   - PromptListChangedNotification: sends notifications/prompts/list_changed
//   - ToolListChangedNotification: sends notifications/tools/list_changed
//
// Notifications are delivered to one active SSE stream (POST preferred, then
// GET). Sending returns an error when no active stream can accept the message.
//
// # Sampling
//
// The handler supports MCP Sampling, allowing the server to request
// LLM completions from the connected client. Tools can call
// StreamableHandler.SendSamplingRequest during execution to request a
// completion. The request is sent via SSE to the client, which responds via
// POST. The tool blocks until the response arrives or the context is cancelled.
// The client must advertise the sampling capability during initialization.
//
// # Pagination
//
// All list methods support cursor-based pagination. Pass a "cursor" string in
// the request params; the server returns a "nextCursor" when more pages are
// available. The default page size is 50 items.
//
// # Error Codes
//
//	-32700  Parse error (invalid JSON)
//	-32601  Method not found
//	-32602  Invalid params (unknown tool, missing prompt, bad argument)
//	-32603  Internal error
//	-32002  Resource not found
//	-32001  Unauthorized (a MethodAuthorizer or Authorizer refused the request)
//	-32021  Missing required client capability (2026-07-28 input requests)
//
// Tool execution failures are CallToolResult values with isError=true. Invalid
// calls, including unknown tools, use JSON-RPC invalid-params errors.
//
// # Quick Start
//
//	rt := interaction.NewRuntime()
//	rt.RegisterTool(myTool)
//	http.Handle("/mcp", mcp.NewHandler(rt))
//	http.ListenAndServe(":8080", nil)
//
// # Streamable HTTP with Sampling
//
//	rt := interaction.NewRuntime()
//	rt.RegisterTool(myTool)
//	h := mcp.NewStreamableHandler(rt)
//	http.Handle("/mcp", h)
//	http.ListenAndServe(":8080", nil)
//
//	// In a tool implementation:
//	sessionID := mcp.SessionIDFromContext(ctx)
//	result, err := h.SendSamplingRequest(ctx, sessionID, mcp.CreateMessageRequest{
//	    Messages:  []mcp.SamplingMessage{{Role: "user", Content: mcp.SamplingContent{Type: "text", Text: "Hello"}}},
//	    MaxTokens: 100,
//	})
package mcp
