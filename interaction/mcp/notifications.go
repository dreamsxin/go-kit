package mcp

import (
	"encoding/json"
	"fmt"
)

// NotificationMessage builds a JSON-RPC notification for the given method and params.
func NotificationMessage(method string, params map[string]any) json.RawMessage {
	msg := map[string]any{
		"jsonrpc": jsonRPCVersion,
		"method":  method,
		"params":  params,
	}
	b, _ := json.Marshal(msg)
	return b
}

// LogNotification sends a logging message notification to the client.
// Level must be one of: debug, info, notice, warning, error, critical, alert, emergency.
func (h *StreamableHandler) LogNotification(sessionID string, level string, data string, logger string) error {
	params := map[string]any{
		"level": level,
		"data":  map[string]any{"type": "text", "text": data},
	}
	if logger != "" {
		params["logger"] = logger
	}
	return h.sendNotification(sessionID, NotificationMessage("notifications/message", params))
}

// ProgressNotification sends a progress update notification to the client.
// progressToken must match the token previously provided by the client in the request.
func (h *StreamableHandler) ProgressNotification(sessionID string, progressToken any, progress, total float64) error {
	params := map[string]any{
		"progressToken": progressToken,
		"progress":      progress,
	}
	if total > 0 {
		params["total"] = total
	}
	return h.sendNotification(sessionID, NotificationMessage("notifications/progress", params))
}

// ResourceUpdatedNotification informs the client that a specific resource has changed.
func (h *StreamableHandler) ResourceUpdatedNotification(sessionID string, uri string) error {
	params := map[string]any{"uri": uri}
	return h.sendNotification(sessionID, NotificationMessage("notifications/resources/updated", params))
}

// ResourceListChangedNotification informs the client that the resource list has changed.
func (h *StreamableHandler) ResourceListChangedNotification(sessionID string) error {
	return h.sendNotification(sessionID, NotificationMessage("notifications/resources/list_changed", map[string]any{}))
}

// PromptListChangedNotification informs the client that the prompt list has changed.
func (h *StreamableHandler) PromptListChangedNotification(sessionID string) error {
	return h.sendNotification(sessionID, NotificationMessage("notifications/prompts/list_changed", map[string]any{}))
}

// ToolListChangedNotification informs the client that the tool list has changed.
func (h *StreamableHandler) ToolListChangedNotification(sessionID string) error {
	return h.sendNotification(sessionID, NotificationMessage("notifications/tools/list_changed", map[string]any{}))
}

// sendNotification delivers a JSON-RPC notification to the client's active SSE stream.
func (h *StreamableHandler) sendNotification(sessionID string, data json.RawMessage) error {
	sess, ok := h.store.get(sessionID)
	if !ok {
		return fmt.Errorf("mcp: session %q not found", sessionID)
	}
	if delivered, err := sess.writeToPOST(data); delivered || err != nil {
		return err
	}
	return sess.broadcastToGET(data)
}
