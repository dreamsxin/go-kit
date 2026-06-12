// Package main demonstrates a full MCP server with tools, resources, prompts,
// notifications, and completions using the Streamable HTTP transport.
//
// Start the server and interact via any MCP client:
//
//	go run ./examples/mcp_full
//
// Basic HTTP interaction (simple POST):
//
//	curl -X POST http://localhost:8090/mcp -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
//	curl -X POST http://localhost:8090/mcp -H 'Mcp-Session-Id: <sid>' -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
//	curl -X POST http://localhost:8090/mcp -H 'Mcp-Session-Id: <sid>' -d '{"jsonrpc":"2.0","id":3,"method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"code_review"},"argument":{"name":"language","value":"go"}}}'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dreamsxin/go-kit/interaction"
	interactionmcp "github.com/dreamsxin/go-kit/interaction/mcp"
)

func main() {
	rt := buildRuntime()

	// Use StreamableHandler for full Streamable HTTP support:
	// POST for JSON-RPC, GET for SSE streams, DELETE for session termination
	h := interactionmcp.NewStreamableHandler(rt)

	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Demo: simulate a notification endpoint that sends tool list changed
	mux.HandleFunc("/demo/notify-tools-changed", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session")
		if sessionID == "" {
			http.Error(w, "session query parameter required", http.StatusBadRequest)
			return
		}
		if err := h.ToolListChangedNotification(sessionID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("notification sent"))
	})

	// Demo: simulate a log notification endpoint
	mux.HandleFunc("/demo/log", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session")
		msg := r.URL.Query().Get("msg")
		if sessionID == "" || msg == "" {
			http.Error(w, "session and msg query parameters required", http.StatusBadRequest)
			return
		}
		if err := h.LogNotification(sessionID, "info", msg, "mcp_full_demo"); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("log notification sent"))
	})

	log.Println("MCP full example listening on :8090")
	log.Println("  POST /mcp           - MCP JSON-RPC endpoint")
	log.Println("  GET  /mcp           - SSE stream for server-initiated messages")
	log.Println("  DELETE /mcp         - terminate session")
	log.Println("  GET  /demo/notify-tools-changed?session=<sid>")
	log.Println("  GET  /demo/log?session=<sid>&msg=<message>")
	log.Fatal(http.ListenAndServe(":8090", mux))
}

func buildRuntime() *interaction.Runtime {
	rt := interaction.NewRuntime(nil, nil, nil)

	// ── Register tools ────────────────────────────────────────────────
	if err := rt.RegisterTool(greetTool()); err != nil {
		panic(err)
	}
	if err := rt.RegisterTool(timeTool()); err != nil {
		panic(err)
	}

	// ── Register resources ────────────────────────────────────────────
	resources := interaction.NewMemoryResourceProvider()
	_ = resources.Register(interaction.Resource{
		URI:         "info://app/name",
		Name:        "app-name",
		Title:       "Application Name",
		Description: "The name of this demo application",
		MIMEType:    "text/plain",
	}, []interaction.ResourceContent{
		{URI: "info://app/name", Text: "go-kit MCP Demo", MIMEType: "text/plain"},
	})
	_ = resources.Register(interaction.Resource{
		URI:         "info://app/version",
		Name:        "app-version",
		Title:       "Application Version",
		Description: "Current version of the demo application",
		MIMEType:    "text/plain",
	}, []interaction.ResourceContent{
		{URI: "info://app/version", Text: "1.0.0", MIMEType: "text/plain"},
	})
	_ = resources.Register(interaction.Resource{
		URI:         "info://app/uptime",
		Name:        "app-uptime",
		Title:       "Server Start Time",
		Description: "The time when the server was started",
		MIMEType:    "application/json",
	}, []interaction.ResourceContent{
		{URI: "info://app/uptime", Text: `{"startedAt":"` + time.Now().UTC().Format(time.RFC3339) + `"}`, MIMEType: "application/json"},
	})
	resources.SetTemplates([]interaction.ResourceTemplate{
		{
			URITemplate: "info://{key}",
			Name:        "app-info",
			Description: "Access any application info value by key (name, version, uptime)",
			MIMEType:    "text/plain",
		},
	})
	rt.WithResources(resources)

	// ── Register prompts ──────────────────────────────────────────────
	prompts := interaction.NewMemoryPromptProvider()
	_ = prompts.Register(interaction.Prompt{
		Name:        "summarize",
		Title:       "Summarize Content",
		Description: "Ask the LLM to summarize a piece of text",
		Arguments: []interaction.PromptArgument{
			{Name: "text", Description: "The text to summarize", Required: true},
			{Name: "max_words", Description: "Maximum word count for the summary"},
		},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		maxWords := args["max_words"]
		if maxWords == "" {
			maxWords = "100"
		}
		return interaction.PromptResult{
			Description: "Summarization prompt",
			Messages: []interaction.PromptMessage{
				{Role: "user", Content: interaction.PromptContent{
					Type: "text",
					Text: fmt.Sprintf("Please summarize the following text in at most %s words:\n\n%s", maxWords, args["text"]),
				}},
			},
		}, nil
	})
	_ = prompts.Register(interaction.Prompt{
		Name:        "code_review",
		Title:       "Code Review",
		Description: "Ask the LLM to review source code",
		Arguments: []interaction.PromptArgument{
			{Name: "code", Description: "The source code to review", Required: true},
			{Name: "language", Description: "Programming language (e.g. go, python, rust)"},
			{Name: "focus", Description: "What to focus on (e.g. security, performance, style)"},
		},
	}, func(args map[string]string) (interaction.PromptResult, error) {
		lang := args["language"]
		if lang == "" {
			lang = "unknown"
		}
		focus := args["focus"]
		if focus == "" {
			focus = "correctness, readability, and best practices"
		}
		return interaction.PromptResult{
			Description: "Code review prompt",
			Messages: []interaction.PromptMessage{
				{Role: "system", Content: interaction.PromptContent{
					Type: "text",
					Text: "You are an expert code reviewer. Focus on " + focus + ".",
				}},
				{Role: "user", Content: interaction.PromptContent{
					Type: "text",
					Text: fmt.Sprintf("Please review this %s code:\n\n```\n%s\n```", lang, args["code"]),
				}},
			},
		}, nil
	})
	rt.WithPrompts(prompts)

	return rt
}

// ── Tool definitions ────────────────────────────────────────────────────────

func greetTool() interaction.Tool {
	return &describedTool{
		name:        "greet",
		description: "Generate a greeting message",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "description": "Person to greet"},
				"style": map[string]any{"type": "string", "description": "Greeting style: formal, casual, or enthusiastic"},
			},
			"required": []string{"name"},
		},
		call: func(_ context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			args, _ := json.Marshal(call.Input)
			var params struct {
				Name  string `json:"name"`
				Style string `json:"style"`
			}
			_ = json.Unmarshal(args, &params)
			if params.Name == "" {
				params.Name = "world"
			}
			var greeting string
			switch params.Style {
			case "formal":
				greeting = fmt.Sprintf("Good day, %s. It is a pleasure to make your acquaintance.", params.Name)
			case "enthusiastic":
				greeting = fmt.Sprintf("Hey %s!!! So awesome to see you today!!!", params.Name)
			default:
				greeting = fmt.Sprintf("Hello, %s! How's it going?", params.Name)
			}
			return interaction.ToolResult{Output: map[string]string{"greeting": greeting}}, nil
		},
	}
}

func timeTool() interaction.Tool {
	return &describedTool{
		name:        "current_time",
		description: "Get the current server time in UTC",
		inputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		call: func(_ context.Context, _ interaction.ToolCall) (interaction.ToolResult, error) {
			now := time.Now().UTC()
			return interaction.ToolResult{Output: map[string]string{
				"time":     now.Format(time.RFC3339),
				"unix":     fmt.Sprintf("%d", now.Unix()),
				"weekday":  now.Weekday().String(),
			}}, nil
		},
	}
}

type describedTool struct {
	name        string
	description string
	inputSchema any
	call        func(context.Context, interaction.ToolCall) (interaction.ToolResult, error)
}

func (t *describedTool) Name() string { return t.name }

func (t *describedTool) Descriptor() interaction.ToolDescriptor {
	return interaction.ToolDescriptor{
		Name:        t.name,
		Description: t.description,
		InputSchema: t.inputSchema,
	}
}

func (t *describedTool) Call(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
	return t.call(ctx, call)
}
