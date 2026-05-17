// Package mcp exposes a preview MCP-style JSON-RPC endpoint for interaction runtimes.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dreamsxin/go-kit/interaction"
)

const jsonRPCVersion = "2.0"

type Handler struct {
	Runtime *interaction.Runtime
}

func NewHandler(runtime *interaction.Runtime) *Handler {
	if runtime == nil {
		runtime = interaction.NewRuntime(nil, nil, nil)
	}
	return &Handler{Runtime: runtime}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeHTTPError(w, http.StatusMethodNotAllowed, "method_not_allowed", "MCP preview endpoint expects POST")
		return
	}
	defer r.Body.Close()

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeResponse(w, response{JSONRPC: jsonRPCVersion, Error: newError(-32700, "parse error", err.Error())})
		return
	}
	resp := h.handle(r.Context(), req)
	writeResponse(w, resp)
}

func (h *Handler) handle(ctx context.Context, req request) response {
	resp := response{JSONRPC: jsonRPCVersion, ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "microgen.interaction.mcp.preview",
			"serverInfo": map[string]any{
				"name":    "go-kit interaction",
				"version": "preview",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": toMCPTools(h.Runtime.ListTools())}
	case "tools/call":
		result, err := h.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32000, "tool call failed", err.Error())
			return resp
		}
		resp.Result = result
	default:
		resp.Error = newError(-32601, "method not found", req.Method)
	}
	return resp
}

func (h *Handler) callTool(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params callParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.Name == "" {
		return nil, errors.New("tool name is required")
	}

	sessionID := interaction.SessionID(params.SessionID)
	if sessionID == "" {
		session, err := h.Runtime.StartSession(ctx, params.Subject, params.Metadata)
		if err != nil {
			return nil, err
		}
		sessionID = session.ID
	}

	result, err := h.Runtime.CallTool(ctx, interaction.ToolCall{
		SessionID: sessionID,
		Name:      params.Name,
		Input:     params.Arguments,
		Metadata:  params.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": fmt.Sprint(result.Output),
		}},
		"structuredContent": result.Output,
		"sessionId":         string(sessionID),
		"metadata":          result.Metadata,
	}, nil
}

func toMCPTools(descriptors []interaction.ToolDescriptor) []map[string]any {
	tools := make([]map[string]any, 0, len(descriptors))
	for _, descriptor := range descriptors {
		tool := map[string]any{"name": descriptor.Name}
		if descriptor.Description != "" {
			tool["description"] = descriptor.Description
		}
		if descriptor.InputSchema != nil {
			tool["inputSchema"] = descriptor.InputSchema
		}
		if len(descriptor.Metadata) > 0 {
			tool["metadata"] = descriptor.Metadata
		}
		tools = append(tools, tool)
	}
	return tools
}

func writeHTTPError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

func writeResponse(w http.ResponseWriter, resp response) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func newError(code int, message, data string) *rpcError {
	return &rpcError{Code: code, Message: message, Data: data}
}

type request struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

type callParams struct {
	SessionID string            `json:"sessionId,omitempty"`
	Subject   string            `json:"subject,omitempty"`
	Name      string            `json:"name"`
	Arguments any               `json:"arguments,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
