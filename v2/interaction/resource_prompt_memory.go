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

// NewMemoryResourceProvider returns an in-memory ResourceProvider suitable for
// tests and small deployments.
func NewMemoryResourceProvider() *MemoryResourceProvider {
	return &MemoryResourceProvider{
		resources: make(map[string]Resource),
		contents:  make(map[string][]ResourceContent),
	}
}

// Register adds a resource and its content to the provider.
// Returns ErrResourceExists if a resource with the same URI is already registered.
func (p *MemoryResourceProvider) Register(res Resource, content []ResourceContent) error {
	if res.URI == "" {
		return fmt.Errorf("interaction: resource URI is required")
	}
	if res.Name == "" {
		return fmt.Errorf("interaction: resource name is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.resources[res.URI]; exists {
		return ErrResourceExists
	}
	p.resources[res.URI] = cloneResource(res)
	p.contents[res.URI] = cloneResourceContents(content)
	return nil
}

// SetTemplates replaces the resource template list.
func (p *MemoryResourceProvider) SetTemplates(templates []ResourceTemplate) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.templates = cloneResourceTemplates(templates)
}

// RegisterText is a convenience method for registering a plain-text resource
// without manually constructing ResourceContent slices.
func (p *MemoryResourceProvider) RegisterText(uri, name, text string) error {
	return p.Register(
		Resource{URI: uri, Name: name, MIMEType: "text/plain"},
		[]ResourceContent{{URI: uri, Text: text, MIMEType: "text/plain"}},
	)
}

// ListResources returns all registered resources sorted by URI.
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

// ReadResource returns the content of the resource identified by uri.
// Returns ErrResourceNotFound if the URI is not registered.
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
	return cloneResourceContents(content), nil
}

// ListResourceTemplates returns all registered resource URI templates.
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
	prompt Prompt
	render func(args map[string]string) (PromptResult, error)
}

// NewMemoryPromptProvider returns an in-memory PromptProvider suitable for
// tests and small deployments.
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
	p.prompts[prompt.Name] = memoryPromptEntry{prompt: clonePrompt(prompt), render: render}
	return nil
}

// ListPrompts returns all registered prompts sorted by name.
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
		out = append(out, clonePrompt(p.prompts[name].prompt))
	}
	return out, nil
}

// GetPrompt renders the named prompt with the given arguments.
// Returns ErrPromptNotFound if the prompt does not exist.
func (p *MemoryPromptProvider) GetPrompt(ctx context.Context, name string, args map[string]string) (PromptResult, error) {
	if err := ctx.Err(); err != nil {
		return PromptResult{}, err
	}
	p.mu.RLock()
	entry, ok := p.prompts[name]
	p.mu.RUnlock()
	if !ok {
		return PromptResult{}, ErrPromptNotFound
	}
	if entry.render == nil {
		return PromptResult{}, fmt.Errorf("interaction: prompt %q has no render function", name)
	}
	return entry.render(cloneStringMap(args))
}

// CompleteArgument provides basic prefix-match completions for registered
// prompt arguments. The caller may supply a custom completer per prompt by
// storing it alongside the prompt entry; this default falls back to empty.
func (p *MemoryPromptProvider) CompleteArgument(ctx context.Context, promptName, argName, partialValue string) (CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return CompletionResult{}, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.prompts[promptName]
	if !ok {
		return CompletionResult{}, ErrPromptNotFound
	}
	_ = argName      // reserved for per-argument completer hooks
	_ = partialValue // default implementation returns no suggestions
	_ = entry
	return CompletionResult{Values: []string{}, Total: 0, HasMore: false}, nil
}

func cloneResource(resource Resource) Resource {
	resource.Metadata = cloneStringMap(resource.Metadata)
	return resource
}

func cloneResourceContents(contents []ResourceContent) []ResourceContent {
	if contents == nil {
		return nil
	}
	out := make([]ResourceContent, len(contents))
	for i, content := range contents {
		content.Blob = append([]byte(nil), content.Blob...)
		out[i] = content
	}
	return out
}

func cloneResourceTemplates(templates []ResourceTemplate) []ResourceTemplate {
	if templates == nil {
		return nil
	}
	out := make([]ResourceTemplate, len(templates))
	for i, template := range templates {
		template.Metadata = cloneStringMap(template.Metadata)
		out[i] = template
	}
	return out
}

func clonePrompt(prompt Prompt) Prompt {
	prompt.Arguments = append([]PromptArgument(nil), prompt.Arguments...)
	return prompt
}
