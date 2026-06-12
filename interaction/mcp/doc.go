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
// # Transport Modes
//
// Two transport modes are available:
//
// Handler (simple POST-only): A basic JSON-RPC endpoint that accepts POST
// requests and returns JSON responses. Suitable for simple integrations where
// SSE streaming and server-initiated requests are not needed.
//
// StreamableHandler (Streamable HTTP): A full MCP transport supporting:
//   - POST for client JSON-RPC messages (requests, notifications, responses)
//   - GET  for persistent SSE streams (server-initiated messages)
//   - DELETE for explicit session termination
//   - Session management via Mcp-Session-Id header
//   - SSE streaming responses when client sends Accept: text/event-stream
//   - Server-initiated requests (sampling/createMessage)
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
// Logging: logging/setLevel — adjust server log verbosity.
//
// # Sampling (StreamableHandler only)
//
// The StreamableHandler supports MCP Sampling, allowing the server to request
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
// # Quick Start (Simple Handler)
//
//	rt := interaction.NewRuntime(nil, nil, nil)
//	rt.RegisterTool(myTool)
//	http.Handle("/mcp", mcp.NewHandler(rt))
//	http.ListenAndServe(":8080", nil)
//
// # Quick Start (Streamable HTTP with Sampling)
//
//	rt := interaction.NewRuntime(nil, nil, nil)
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
