package interaction

import (
	"context"
	"sort"
	"sync"
)

// MemoryToolRegistry is an in-memory ToolRegistry.
type MemoryToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewMemoryToolRegistry returns an in-memory ToolRegistry suitable for tests
// and lightweight usage.
func NewMemoryToolRegistry() *MemoryToolRegistry {
	return &MemoryToolRegistry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Returns ErrToolExists if a tool with
// the same name is already registered.
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

// Get returns the tool with the given name, or false if not found.
func (r *MemoryToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Call executes the named tool. Returns ErrToolNotFound if the tool does not
// exist.
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

// List returns descriptors for all registered tools, sorted by name.
func (r *MemoryToolRegistry) List() []ToolDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := make([]ToolDescriptor, 0, len(r.tools))
	for _, name := range names {
		tool := r.tools[name]
		if describer, ok := tool.(ToolDescriber); ok {
			descriptor := describer.Descriptor()
			if descriptor.Name == "" {
				descriptor.Name = name
			}
			descriptor.Metadata = cloneStringMap(descriptor.Metadata)
			tools = append(tools, descriptor)
			continue
		}
		tools = append(tools, ToolDescriptor{Name: name})
	}
	return tools
}
