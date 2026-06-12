package interaction

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryResourceProvider is an in-memory ResourceProvider for tests and small deployments.
type MemoryResourceProvider struct {
	mu        sync.RWMutex
	resources map[string]Resource
	contents  map[string][]ResourceContent
	templates []ResourceTemplate
}

func NewMemoryResourceProvider() *MemoryResourceProvider {
	return &MemoryResourceProvider{
		resources: make(map[string]Resource),
		contents:  make(map[string][]ResourceContent),
	}
}

// Register adds a resource and its content to the provider.
func (p *MemoryResourceProvider) Register(res Resource, content []ResourceContent) error {
	if res.URI == "" {
		return fmt.Errorf("interaction: resource URI is required")
	}
	if res.Name == "" {
		return fmt.Errorf("interaction: resource name is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resources[res.URI] = res
	p.contents[res.URI] = content
	return nil
}

// SetTemplates replaces the resource template list.
func (p *MemoryResourceProvider) SetTemplates(templates []ResourceTemplate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.templates = append([]ResourceTemplate(nil), templates...)
}

func (p *MemoryResourceProvider) ListResources(ctx context.Context) ([]Resource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	uris := make([]string, 0, len(p.resources))
	for uri := range p.resources {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	out := make([]Resource, 0, len(uris))
	for _, uri := range uris {
		res := p.resources[uri]
		res.Metadata = cloneStringMap(res.Metadata)
		out = append(out, res)
	}
	return out, nil
}

func (p *MemoryResourceProvider) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	content, ok := p.contents[uri]
	if !ok {
		return nil, ErrResourceNotFound
	}
	out := make([]ResourceContent, len(content))
	copy(out, content)
	return out, nil
}

func (p *MemoryResourceProvider) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]ResourceTemplate, len(p.templates))
	for i, t := range p.templates {
		t.Metadata = cloneStringMap(t.Metadata)
		out[i] = t
	}
	return out, nil
}

// MemoryPromptProvider is an in-memory PromptProvider for tests and small deployments.
type MemoryPromptProvider struct {
	mu      sync.RWMutex
	prompts map[string]memoryPromptEntry
}

type memoryPromptEntry struct {
	prompt  Prompt
	render  func(args map[string]string) (PromptResult, error)
}

func NewMemoryPromptProvider() *MemoryPromptProvider {
	return &MemoryPromptProvider{
		prompts: make(map[string]memoryPromptEntry),
	}
}

// Register adds a prompt template with its render function.
func (p *MemoryPromptProvider) Register(prompt Prompt, render func(args map[string]string) (PromptResult, error)) error {
	if prompt.Name == "" {
		return fmt.Errorf("interaction: prompt name is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.prompts[prompt.Name]; exists {
		return ErrPromptExists
	}
	p.prompts[prompt.Name] = memoryPromptEntry{prompt: prompt, render: render}
	return nil
}

func (p *MemoryPromptProvider) ListPrompts(ctx context.Context) ([]Prompt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	names := make([]string, 0, len(p.prompts))
	for name := range p.prompts {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Prompt, 0, len(names))
	for _, name := range names {
		out = append(out, p.prompts[name].prompt)
	}
	return out, nil
}

func (p *MemoryPromptProvider) GetPrompt(ctx context.Context, name string, args map[string]string) (PromptResult, error) {
	if err := ctx.Err(); err != nil {
		return PromptResult{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.prompts[name]
	if !ok {
		return PromptResult{}, ErrPromptNotFound
	}
	if entry.render == nil {
		return PromptResult{}, fmt.Errorf("interaction: prompt %q has no render function", name)
	}
	return entry.render(args)
}
