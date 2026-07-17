// Package mcp exposes a Model Context Protocol (MCP) compliant JSON-RPC 2.0
// HTTP endpoint for go-kit services. It bridges the interaction.Runtime
// (tools, resources, prompts) with any MCP-capable AI client.
//
// # Protocol Conformance
//
// The server implements the MCP specification dated 2025-06-18. It advertises
// protocolVersion "2025-06-18" during the initialize handshake and declares
// capabilities dynamically based on which providers are attached to the runtime.
//
// # Transport
//
// StreamableHandler implements the full MCP Streamable HTTP transport:
//   - POST for client JSON-RPC messages (requests, notifications, responses)
//   - GET  for persistent SSE streams (server-initiated messages)
//   - DELETE for explicit session termination
//   - Session management via Mcp-Session-Id header
//   - SSE streaming responses when client sends Accept: text/event-stream
//   - Server-initiated requests (sampling/createMessage)
//
// NewHandler is a convenience alias for NewStreamableHandler.
//
// # Supported Methods
//
// Base protocol: initialize, notifications/initialized, ping.
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
// Notifications are delivered to the client's active SSE stream (POST or GET).
// If no active stream exists, the notification is silently dropped.
//
// # Sampling
//
// The handler supports MCP Sampling, allowing the server to request
// LLM completions from the connected client. Tools can call
// StreamableHandler.SendSamplingRequest during execution to request a
// completion. The request is sent via SSE to the client, which responds via
// POST. The tool blocks until the response arrives or the context is cancelled.
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
//	-32000  Tool call failed (application-level)
//	-32002  Resource not found
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
//	result, err := h.SendSamplingRequest(ctx, sessionID, mcp.CreateMessageRequest{
//	    Messages:  []mcp.SamplingMessage{{Role: "user", Content: mcp.SamplingContent{Type: "text", Text: "Hello"}}},
//	    MaxTokens: 100,
//	})
package mcp
