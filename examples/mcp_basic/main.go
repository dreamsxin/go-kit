// Package main demonstrates the simplest possible MCP server using go-kit's
// interaction runtime and Streamable HTTP transport.
//
// Run:
//
//	go run ./examples/mcp_basic
//
// Test with curl:
//
//	# Initialize a session (note the Mcp-Session-Id response header)
//	curl -i -X POST http://localhost:8080/mcp \
//	  -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
//
//	# List tools (replace <sid> with the session ID from above)
//	curl -X POST http://localhost:8080/mcp \
//	  -H 'Mcp-Session-Id: <sid>' \
//	  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
//
//	# Call the greet tool
//	curl -X POST http://localhost:8080/mcp \
//	  -H 'Mcp-Session-Id: <sid>' \
//	  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"greet","arguments":{"name":"World"}}}'
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dreamsxin/go-kit/interaction"
	"github.com/dreamsxin/go-kit/interaction/mcp"
)

func main() {
	rt := interaction.NewRuntime()

	_ = rt.RegisterTool(interaction.ToolFunc{
		ToolName:    "greet",
		Description: "Returns a greeting for the given name.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Who to greet"},
			},
		},
		Fn: func(_ context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			args, _ := call.Input.(map[string]any)
			name, _ := args["name"].(string)
			if name == "" {
				name = "World"
			}
			return interaction.ToolResult{
				Output: fmt.Sprintf("Hello, %s!", name),
			}, nil
		},
	})

	log.Println("mcp_basic listening on :8080")
	log.Fatal(mcp.ListenAndServe(":8080", rt))
}
