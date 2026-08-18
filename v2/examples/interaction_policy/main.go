// Package main demonstrates authorization and audit hooks on the go-kit
// interaction runtime, exposed through the MCP Streamable HTTP transport.
//
// The runtime registers a single "echo" tool and installs two hooks:
// an AuthorizationHook whose Authorizer allows only "echo", and an AuditHook
// that records every allowed call to an in-memory sink. Tool calls outside the
// allowlist are rejected before execution and never reach the audit sink.
//
// Run:
//
//	go run ./examples/interaction_policy
//
// Test with curl:
//
//	# Initialize a session (note the Mcp-Session-Id response header)
//	curl -i -X POST http://localhost:8080/mcp \
//	  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}'
//
//	# Complete the MCP lifecycle before sending requests
//	curl -X POST http://localhost:8080/mcp \
//	  -H 'Mcp-Session-Id: <sid>' \
//	  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}'
//
//	# Call the allowed echo tool (audited: one before and one after record)
//	curl -X POST http://localhost:8080/mcp \
//	  -H 'Mcp-Session-Id: <sid>' \
//	  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}'
//
//	# Call a tool outside the allowlist (rejected, produces no audit records)
//	curl -X POST http://localhost:8080/mcp \
//	  -H 'Mcp-Session-Id: <sid>' \
//	  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"delete_all"}}'
//
//	# Review the captured audit records
//	curl http://localhost:8080/audit
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/dreamsxin/go-kit/v2/interaction"
	interactionmcp "github.com/dreamsxin/go-kit/v2/interaction/mcp"
)

func main() {
	rt, audits := newRuntime()

	mux := http.NewServeMux()
	mux.Handle("/mcp", interactionmcp.NewHandler(rt))
	mux.HandleFunc("/audit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(audits.List())
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	log.Println("interaction policy example listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func newRuntime() (*interaction.Runtime, *memoryAuditSink) {
	audits := &memoryAuditSink{}
	rt := interaction.NewRuntime().WithHooks(
		interaction.AuthorizationHook{Authorizer: allowTools("echo")},
		interaction.AuditHook{Sink: audits},
	)
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName:    "echo",
		Description: "Echoes the provided arguments.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
		Fn: func(_ context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			return interaction.ToolResult{
				Output:   call.Input,
				Metadata: map[string]string{"tool": call.Name},
			}, nil
		},
	}); err != nil {
		panic(err)
	}
	return rt, audits
}

func allowTools(names ...string) interaction.AuthorizerFunc {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	return func(_ context.Context, _ interaction.Session, call interaction.ToolCall) (interaction.AuthorizationDecision, error) {
		if _, ok := allowed[call.Name]; ok {
			return interaction.AuthorizationDecision{Allowed: true}, nil
		}
		return interaction.AuthorizationDecision{Allowed: false, Reason: "tool is not allowed"}, nil
	}
}

type memoryAuditSink struct {
	mu      sync.Mutex
	records []interaction.AuditRecord
}

func (s *memoryAuditSink) RecordAudit(_ context.Context, record interaction.AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *memoryAuditSink) List() []interaction.AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]interaction.AuditRecord, len(s.records))
	copy(out, s.records)
	return out
}
