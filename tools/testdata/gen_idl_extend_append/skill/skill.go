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
				Name:        "CreateUser",
				Description: "CreateUser creates a new user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"username": map[string]interface{}{
							"type": "string",
							"description": "",
						},
						"email": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"username",
						"email",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "GetUser",
				Description: "GetUser retrieves a user by ID.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
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
				Name:        "ListUsers",
				Description: "ListUsers lists all users.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"page": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"keyword": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"page",
						"keyword",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "DeleteUser",
				Description: "DeleteUser removes a user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
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
				Name:        "UpdateUser",
				Description: "UpdateUser modifies a user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"username": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"id",
						"username",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "FindByEmail",
				Description: "FindByEmail finds users by email prefix.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
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
				Name:        "SearchUsers",
				Description: "SearchUsers searches users.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"page": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"keyword": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"page",
						"keyword",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "QueryStats",
				Description: "QueryStats returns statistics.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
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
				Name:        "RemoveExpired",
				Description: "RemoveExpired removes expired users.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
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
				Name:        "EditProfile",
				Description: "EditProfile edits profile.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"username": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"id",
						"username",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "ModifyEmail",
				Description: "ModifyEmail modifies email.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"username": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"id",
						"username",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "PatchStatus",
				Description: "PatchStatus patches status.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
						"username": map[string]interface{}{
							"type": "string",
							"description": "",
						},
					},
					"required": []string{
						"id",
						"username",
					},
				},
			},
		},
		{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "PlaceOrder",
				Description: "PlaceOrder",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"user_id": map[string]interface{}{
							"type": "integer",
							"description": "",
						},
					},
					"required": []string{
						"user_id",
					},
				},
			},
		},
	}
}

func getMCPTools() []MCPTool {
	return []MCPTool{
		{
			Name:        "CreateUser",
			Description: "CreateUser creates a new user.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"username": map[string]interface{}{
						"type": "string",
						"description": "",
					},
					"email": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"username",
					"email",
				},
			},
		},
		{
			Name:        "GetUser",
			Description: "GetUser retrieves a user by ID.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "ListUsers",
			Description: "ListUsers lists all users.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"page": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"keyword": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"page",
					"keyword",
				},
			},
		},
		{
			Name:        "DeleteUser",
			Description: "DeleteUser removes a user.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "UpdateUser",
			Description: "UpdateUser modifies a user.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"username": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"id",
					"username",
				},
			},
		},
		{
			Name:        "FindByEmail",
			Description: "FindByEmail finds users by email prefix.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "SearchUsers",
			Description: "SearchUsers searches users.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"page": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"keyword": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"page",
					"keyword",
				},
			},
		},
		{
			Name:        "QueryStats",
			Description: "QueryStats returns statistics.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "RemoveExpired",
			Description: "RemoveExpired removes expired users.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"id",
				},
			},
		},
		{
			Name:        "EditProfile",
			Description: "EditProfile edits profile.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"username": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"id",
					"username",
				},
			},
		},
		{
			Name:        "ModifyEmail",
			Description: "ModifyEmail modifies email.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"username": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"id",
					"username",
				},
			},
		},
		{
			Name:        "PatchStatus",
			Description: "PatchStatus patches status.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
					"username": map[string]interface{}{
						"type": "string",
						"description": "",
					},
				},
				"required": []string{
					"id",
					"username",
				},
			},
		},
		{
			Name:        "PlaceOrder",
			Description: "PlaceOrder",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_id": map[string]interface{}{
						"type": "integer",
						"description": "",
					},
				},
				"required": []string{
					"user_id",
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
