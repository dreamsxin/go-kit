// Package mcp exposes an MCP-compliant JSON-RPC endpoint for interaction runtimes.
//
// The handler implements the Model Context Protocol (2025-06-18) server surface:
// tools, resources, prompts, logging, ping, and capability negotiation.
// See doc.go for full transport and protocol documentation.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

const (
	jsonRPCVersion  = "2.0"
	protocolVersion = "2025-06-18"
	serverName      = "go-kit interaction"
	serverTitle     = "Go Kit Interaction MCP Server"
	defaultPageSize = 50
	serverVersion   = "0.4.0"
)

// ─── shared dispatch core ────────────────────────────────────────────────────
//
// dispatchCore contains the method dispatch logic shared by the handler.

type dispatchCore struct {
	Runtime  *interaction.Runtime
	logLevel string
	mu       sync.RWMutex
}

func (c *dispatchCore) dispatch(ctx context.Context, req request) response {
	resp := response{JSONRPC: jsonRPCVersion, ID: req.ID}
	switch req.Method {
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = c.handleToolsList(ctx, req.Params)
	case "tools/call":
		result, err := c.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32000, "tool call failed", err.Error())
			return resp
		}
		resp.Result = result
	case "resources/list":
		result, err := c.handleResourcesList(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32603, "internal error", err.Error())
			return resp
		}
		resp.Result = result
	case "resources/read":
		result, err := c.handleResourcesRead(ctx, req.Params)
		if err != nil {
			resp.Error = resourceError(err)
			return resp
		}
		resp.Result = result
	case "resources/templates/list":
		result, err := c.handleResourceTemplatesList(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32603, "internal error", err.Error())
			return resp
		}
		resp.Result = result
	case "prompts/list":
		result, err := c.handlePromptsList(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32603, "internal error", err.Error())
			return resp
		}
		resp.Result = result
	case "prompts/get":
		result, err := c.handlePromptsGet(ctx, req.Params)
		if err != nil {
			resp.Error = promptError(err)
			return resp
		}
		resp.Result = result
	case "logging/setLevel":
		result, err := c.handleLoggingSetLevel(req.Params)
		if err != nil {
			resp.Error = newError(-32602, "invalid argument", err.Error())
			return resp
		}
		resp.Result = result
	case "completion/complete":
		result, err := c.handleCompletionComplete(ctx, req.Params)
		if err != nil {
			resp.Error = newError(-32602, "invalid argument", err.Error())
			return resp
		}
		resp.Result = result
	default:
		resp.Error = newError(-32601, "method not found", req.Method)
	}
	return resp
}

func (c *dispatchCore) buildCapabilities() map[string]any {
	caps := map[string]any{
		"tools":   map[string]any{"listChanged": true},
		"logging": map[string]any{},
	}
	if c.Runtime.Resources != nil {
		caps["resources"] = map[string]any{"subscribe": false, "listChanged": true}
	}
	if c.Runtime.Prompts != nil {
		caps["prompts"] = map[string]any{"listChanged": true}
		if _, ok := c.Runtime.Prompts.(interaction.PromptCompleter); ok {
			caps["completions"] = map[string]any{}
		}
	}
	return caps
}

func (c *dispatchCore) buildInitializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"serverInfo": map[string]any{
			"name":    serverName,
			"title":   serverTitle,
			"version": serverVersion,
		},
		"capabilities": c.buildCapabilities(),
		"instructions": "Expose go-kit service methods as MCP tools, resources, and prompts.",
	}
}

// ─── tools ───────────────────────────────────────────────────────────────────

func (c *dispatchCore) handleToolsList(ctx context.Context, raw json.RawMessage) map[string]any {
	cursor, _ := parseCursor(raw)
	all := c.Runtime.ListTools()
	page, next := paginate(cursor, len(all), defaultPageSize, func(i int) map[string]any {
		return toMCPTool(all[i])
	})
	result := map[string]any{"tools": page}
	if next != "" {
		result["nextCursor"] = next
	}
	return result
}

func (c *dispatchCore) callTool(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
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
		session, err := c.Runtime.StartSession(ctx, params.Subject, params.Metadata)
		if err != nil {
			return nil, err
		}
		sessionID = session.ID
		// Auto-created sessions are scoped to this tool call only.
		// Close them when we're done to avoid leaking sessions in the store.
		defer func() {
			_, _ = c.Runtime.EndSession(ctx, sessionID)
		}()
	}

	// Propagate session ID through context so tool implementations can
	// access it for notifications, sampling, or audit.
	ctx = context.WithValue(ctx, sessionIDContextKey{}, string(sessionID))

	result, err := c.Runtime.CallTool(ctx, interaction.ToolCall{
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

// ─── resources ───────────────────────────────────────────────────────────────

func (c *dispatchCore) handleResourcesList(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	if c.Runtime.Resources == nil {
		return map[string]any{"resources": []any{}}, nil
	}
	cursor, _ := parseCursor(raw)
	all, err := c.Runtime.ListResources(ctx)
	if err != nil {
		return nil, err
	}
	page, next := paginate(cursor, len(all), defaultPageSize, func(i int) map[string]any {
		return toMCPResource(all[i])
	})
	result := map[string]any{"resources": page}
	if next != "" {
		result["nextCursor"] = next
	}
	return result, nil
}

func (c *dispatchCore) handleResourcesRead(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params struct {
		URI string `json:"uri"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.URI == "" {
		return nil, errors.New("resource uri is required")
	}
	contents, err := c.Runtime.ReadResource(ctx, params.URI)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(contents))
	for _, ct := range contents {
		item := map[string]any{"uri": ct.URI}
		if ct.MIMEType != "" {
			item["mimeType"] = ct.MIMEType
		}
		if len(ct.Blob) > 0 {
			item["blob"] = ct.Blob
		} else {
			item["text"] = ct.Text
		}
		items = append(items, item)
	}
	return map[string]any{"contents": items}, nil
}

func (c *dispatchCore) handleResourceTemplatesList(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	if c.Runtime.Resources == nil {
		return map[string]any{"resourceTemplates": []any{}}, nil
	}
	cursor, _ := parseCursor(raw)
	all, err := c.Runtime.ListResourceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	page, next := paginate(cursor, len(all), defaultPageSize, func(i int) map[string]any {
		return toMCPResourceTemplate(all[i])
	})
	result := map[string]any{"resourceTemplates": page}
	if next != "" {
		result["nextCursor"] = next
	}
	return result, nil
}

// ─── prompts ─────────────────────────────────────────────────────────────────

func (c *dispatchCore) handlePromptsList(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	if c.Runtime.Prompts == nil {
		return map[string]any{"prompts": []any{}}, nil
	}
	cursor, _ := parseCursor(raw)
	all, err := c.Runtime.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	page, next := paginate(cursor, len(all), defaultPageSize, func(i int) map[string]any {
		return toMCPPrompt(all[i])
	})
	result := map[string]any{"prompts": page}
	if next != "" {
		result["nextCursor"] = next
	}
	return result, nil
}

func (c *dispatchCore) handlePromptsGet(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments,omitempty"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.Name == "" {
		return nil, errors.New("prompt name is required")
	}
	result, err := c.Runtime.GetPrompt(ctx, params.Name, params.Arguments)
	if err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0, len(result.Messages))
	for _, m := range result.Messages {
		msg := map[string]any{"role": m.Role}
		content := map[string]any{"type": m.Content.Type}
		switch m.Content.Type {
		case "text":
			content["text"] = m.Content.Text
		case "image", "audio":
			content["data"] = m.Content.Data
			if m.Content.MIMEType != "" {
				content["mimeType"] = m.Content.MIMEType
			}
		case "resource":
			content["uri"] = m.Content.Data
		}
		msg["content"] = content
		messages = append(messages, msg)
	}
	out := map[string]any{"messages": messages}
	if result.Description != "" {
		out["description"] = result.Description
	}
	return out, nil
}

// ─── logging ─────────────────────────────────────────────────────────────────

func (c *dispatchCore) handleLoggingSetLevel(raw json.RawMessage) (map[string]any, error) {
	var params struct {
		Level string `json:"level"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	validLevels := map[string]bool{
		"debug": true, "info": true, "notice": true, "warning": true,
		"error": true, "critical": true, "alert": true, "emergency": true,
	}
	if !validLevels[params.Level] {
		return nil, fmt.Errorf("invalid log level %q", params.Level)
	}
	c.mu.Lock()
	c.logLevel = params.Level
	c.mu.Unlock()
	return map[string]any{}, nil
}

// ─── completions ─────────────────────────────────────────────────────────────

func (c *dispatchCore) handleCompletionComplete(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params struct {
		Ref struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"ref"`
		Argument struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"argument"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("decode params: %w", err)
		}
	}
	if params.Ref.Type == "" || params.Ref.Name == "" {
		return nil, errors.New("ref type and name are required")
	}
	if params.Argument.Name == "" {
		return nil, errors.New("argument name is required")
	}

	var result interaction.CompletionResult
	var err error

	switch params.Ref.Type {
	case "ref/prompt":
		result, err = c.Runtime.CompletePromptArgument(ctx, params.Ref.Name, params.Argument.Name, params.Argument.Value)
	default:
		return nil, fmt.Errorf("unsupported ref type %q", params.Ref.Type)
	}
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"completion": map[string]any{
			"values":  result.Values,
			"total":   result.Total,
			"hasMore": result.HasMore,
		},
		"ref": map[string]any{
			"type": params.Ref.Type,
			"name": params.Ref.Name,
		},
	}, nil
}

// ─── MCP format helpers ──────────────────────────────────────────────────────

func toMCPTool(d interaction.ToolDescriptor) map[string]any {
	tool := map[string]any{"name": d.Name}
	if d.Description != "" {
		tool["description"] = d.Description
	}
	if d.InputSchema != nil {
		tool["inputSchema"] = d.InputSchema
	}
	if len(d.Metadata) > 0 {
		tool["metadata"] = d.Metadata
	}
	return tool
}

func toMCPResource(r interaction.Resource) map[string]any {
	m := map[string]any{"uri": r.URI, "name": r.Name}
	if r.Title != "" {
		m["title"] = r.Title
	}
	if r.Description != "" {
		m["description"] = r.Description
	}
	if r.MIMEType != "" {
		m["mimeType"] = r.MIMEType
	}
	if r.Size > 0 {
		m["size"] = r.Size
	}
	if len(r.Metadata) > 0 {
		m["metadata"] = r.Metadata
	}
	return m
}

func toMCPResourceTemplate(t interaction.ResourceTemplate) map[string]any {
	m := map[string]any{"uriTemplate": t.URITemplate, "name": t.Name}
	if t.Title != "" {
		m["title"] = t.Title
	}
	if t.Description != "" {
		m["description"] = t.Description
	}
	if t.MIMEType != "" {
		m["mimeType"] = t.MIMEType
	}
	if len(t.Metadata) > 0 {
		m["metadata"] = t.Metadata
	}
	return m
}

func toMCPPrompt(p interaction.Prompt) map[string]any {
	m := map[string]any{"name": p.Name}
	if p.Title != "" {
		m["title"] = p.Title
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if len(p.Arguments) > 0 {
		args := make([]map[string]any, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			arg := map[string]any{"name": a.Name}
			if a.Description != "" {
				arg["description"] = a.Description
			}
			if a.Required {
				arg["required"] = true
			}
			args = append(args, arg)
		}
		m["arguments"] = args
	}
	return m
}

// ─── pagination ──────────────────────────────────────────────────────────────

func parseCursor(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var params struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(raw, &params)
	return params.Cursor, params.Cursor != ""
}

func paginate(cursor string, total, pageSize int, render func(int) map[string]any) ([]map[string]any, string) {
	offset := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil && n >= 0 && n < total {
			offset = n
		}
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	items := make([]map[string]any, 0, end-offset)
	for i := offset; i < end; i++ {
		items = append(items, render(i))
	}
	var next string
	if end < total {
		next = strconv.Itoa(end)
	}
	return items, next
}

// ─── error helpers ───────────────────────────────────────────────────────────

func resourceError(err error) *rpcError {
	if errors.Is(err, interaction.ErrResourceNotFound) {
		return newError(-32002, "resource not found", err.Error())
	}
	return newError(-32603, "internal error", err.Error())
}

func promptError(err error) *rpcError {
	if errors.Is(err, interaction.ErrPromptNotFound) {
		return newError(-32602, "prompt not found", err.Error())
	}
	if errors.Is(err, interaction.ErrInvalidArgument) {
		return newError(-32602, "invalid argument", err.Error())
	}
	return newError(-32603, "internal error", err.Error())
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

// ─── wire types ──────────────────────────────────────────────────────────────

// sessionIDContextKey is the context key for the MCP session ID.
type sessionIDContextKey struct{}

// SessionIDFromContext retrieves the MCP session ID stored in the context
// during tool execution. Returns an empty string if no session ID is present.
// Tool implementations can use this to send notifications or sampling requests.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDContextKey{}).(string); ok {
		return v
	}
	return ""
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
