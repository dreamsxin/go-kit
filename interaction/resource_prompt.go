package interaction

import "context"

// Resource is a server-exposed data artifact that MCP clients can read.
type Resource struct {
	URI         string
	Name        string
	Title       string
	Description string
	MIMEType    string
	Size        int64
	Metadata    map[string]string
}

// ResourceContent is the payload returned when reading a resource.
type ResourceContent struct {
	URI      string
	MIMEType string
	Text     string
	Blob     []byte
}

// ResourceTemplate describes a parameterised resource URI pattern.
type ResourceTemplate struct {
	URITemplate string
	Name        string
	Title       string
	Description string
	MIMEType    string
	Metadata    map[string]string
}

// ResourceProvider exposes resources to the interaction runtime.
type ResourceProvider interface {
	ListResources(ctx context.Context) ([]Resource, error)
	ReadResource(ctx context.Context, uri string) ([]ResourceContent, error)
}

// ResourceTemplateLister is implemented by providers that expose URI templates.
type ResourceTemplateLister interface {
	ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error)
}

// PromptArgument describes one named argument accepted by a prompt.
type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

// PromptMessage is a single pre-filled message inside a rendered prompt.
type PromptMessage struct {
	Role    string // "user" | "assistant" | "system"
	Content PromptContent
}

// PromptContent carries the body of a prompt message.
type PromptContent struct {
	Type     string // "text" | "image" | "audio" | "resource"
	Text     string
	MIMEType string
	Data     string // base64 for image/audio, URI for resource
}

// Prompt is a reusable prompt template exposed via MCP.
type Prompt struct {
	Name        string
	Title       string
	Description string
	Arguments   []PromptArgument
}

// PromptResult is the rendered output of a prompt.
type PromptResult struct {
	Description string
	Messages    []PromptMessage
}

// PromptProvider exposes prompt templates to the interaction runtime.
type PromptProvider interface {
	ListPrompts(ctx context.Context) ([]Prompt, error)
	GetPrompt(ctx context.Context, name string, args map[string]string) (PromptResult, error)
}
