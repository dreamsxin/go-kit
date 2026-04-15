package skill

import (
	"encoding/json"
	"net/http"
)

// OpenAI Tool format
type OpenAITool struct {
	Type     string           `json:"type"`
	Function OpenAIFunction   `json:"function"`
}

type OpenAIFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// MCP Tool format
type MCPTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"inputSchema,omitempty"`
}

func getOpenAITools() []OpenAITool {
	return []OpenAITool{
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "GetUser",
				Description: "GetUser retrieves a user by ID.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"id",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "CreateUser",
				Description: "CreateUser creates a new user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
							"description": "",
						},
						"email": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"name",
						"email",
					},
				},
			},
		},
	}
}

func getMCPTools() []MCPTool {
	return []MCPTool{
		{
			Name:        "GetUser",
			Description: "GetUser retrieves a user by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "CreateUser",
			Description: "CreateUser creates a new user.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
						"description": "",
					},
					"email": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"name",
					"email",
				},
			},
		},
	}
}

// Handler returns the AI skill definition in JSON format.
// Supports both OpenAI (via ?format=openai) and MCP (via ?format=mcp) styles.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	format := r.URL.Query().Get("format")
	if format == "mcp" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tools": getMCPTools(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": getOpenAITools(),
	})
}
