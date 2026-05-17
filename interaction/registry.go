package interaction

import (
	"context"
	"sync"
)

// MemoryToolRegistry is an in-memory ToolRegistry.
type MemoryToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewMemoryToolRegistry() *MemoryToolRegistry {
	return &MemoryToolRegistry{tools: make(map[string]Tool)}
}

func (r *MemoryToolRegistry) Register(tool Tool) error {
	if tool == nil {
		return ErrNilTool
	}
	name := tool.Name()
	if name == "" {
		return ErrEmptyToolName
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return ErrToolExists
	}
	r.tools[name] = tool
	return nil
}

func (r *MemoryToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *MemoryToolRegistry) Call(ctx context.Context, call ToolCall) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	tool, ok := r.Get(call.Name)
	if !ok {
		return ToolResult{}, ErrToolNotFound
	}
	return tool.Call(ctx, call)
}
