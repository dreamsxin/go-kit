package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/dreamsxin/go-kit/interaction"
	interactionmcp "github.com/dreamsxin/go-kit/interaction/mcp"
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
	rt := interaction.NewRuntime(nil, nil, nil,
		interaction.AuthorizationHook{Authorizer: allowTools("echo")},
		interaction.AuditHook{Sink: audits},
	)
	if err := rt.RegisterTool(describedEchoTool{}); err != nil {
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

type describedEchoTool struct{}

func (describedEchoTool) Name() string { return "echo" }

func (describedEchoTool) Descriptor() interaction.ToolDescriptor {
	return interaction.ToolDescriptor{
		Name:        "echo",
		Description: "Echoes the provided arguments.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
		},
	}
}

func (describedEchoTool) Call(_ context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
	return interaction.ToolResult{
		Output:   call.Input,
		Metadata: map[string]string{"tool": call.Name},
	}, nil
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
