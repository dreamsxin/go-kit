package interaction

import (
	"context"
	"testing"
)

// ─── MemoryResourceProvider ──────────────────────────────────────────────────

func TestMemoryResourceProvider_RegisterAndList(t *testing.T) {
	p := NewMemoryResourceProvider()
	if err := p.Register(Resource{URI: "test://a", Name: "a"}, []ResourceContent{{URI: "test://a", Text: "alpha"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register(Resource{URI: "test://b", Name: "b"}, []ResourceContent{{URI: "test://b", Text: "beta"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resources, err := p.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(resources))
	}
	// Should be sorted by URI.
	if resources[0].URI != "test://a" || resources[1].URI != "test://b" {
		t.Fatalf("unexpected order: %+v", resources)
	}
}

func TestMemoryResourceProvider_RegisterValidation(t *testing.T) {
	p := NewMemoryResourceProvider()
	if err := p.Register(Resource{}, nil); err == nil {
		t.Fatal("expected error for empty URI")
	}
	if err := p.Register(Resource{URI: "x"}, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestMemoryResourceProvider_ReadResource(t *testing.T) {
	p := NewMemoryResourceProvider()
	_ = p.Register(Resource{URI: "test://a", Name: "a"}, []ResourceContent{
		{URI: "test://a", Text: "hello", MIMEType: "text/plain"},
	})

	contents, err := p.ReadResource(context.Background(), "test://a")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(contents) != 1 || contents[0].Text != "hello" {
		t.Fatalf("unexpected contents: %+v", contents)
	}
}

func TestMemoryResourceProvider_ReadNotFound(t *testing.T) {
	p := NewMemoryResourceProvider()
	_, err := p.ReadResource(context.Background(), "test://missing")
	if err != ErrResourceNotFound {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestMemoryResourceProvider_Templates(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.SetTemplates([]ResourceTemplate{
		{URITemplate: "test://{key}", Name: "dynamic"},
	})
	templates, err := p.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].URITemplate != "test://{key}" {
		t.Fatalf("unexpected templates: %+v", templates)
	}
}

// ─── MemoryPromptProvider ────────────────────────────────────────────────────

func TestMemoryPromptProvider_RegisterAndList(t *testing.T) {
	p := NewMemoryPromptProvider()
	_ = p.Register(Prompt{Name: "greet", Description: "Greet the user"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{Messages: []PromptMessage{{Role: "user", Content: PromptContent{Type: "text", Text: "Hello!"}}}}, nil
	})
	_ = p.Register(Prompt{Name: "bye"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{}, nil
	})

	prompts, err := p.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(prompts))
	}
	if prompts[0].Name != "bye" || prompts[1].Name != "greet" {
		t.Fatalf("unexpected order: %+v", prompts)
	}
}

func TestMemoryPromptProvider_RegisterValidation(t *testing.T) {
	p := NewMemoryPromptProvider()
	if err := p.Register(Prompt{}, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestMemoryPromptProvider_DuplicateName(t *testing.T) {
	p := NewMemoryPromptProvider()
	_ = p.Register(Prompt{Name: "x"}, func(args map[string]string) (PromptResult, error) { return PromptResult{}, nil })
	if err := p.Register(Prompt{Name: "x"}, nil); err != ErrPromptExists {
		t.Fatalf("expected ErrPromptExists, got %v", err)
	}
}

func TestMemoryPromptProvider_GetPrompt(t *testing.T) {
	p := NewMemoryPromptProvider()
	_ = p.Register(Prompt{
		Name:      "review",
		Arguments: []PromptArgument{{Name: "code", Required: true}},
	}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{
			Description: "review prompt",
			Messages:    []PromptMessage{{Role: "user", Content: PromptContent{Type: "text", Text: "Review: " + args["code"]}}},
		}, nil
	})

	result, err := p.GetPrompt(context.Background(), "review", map[string]string{"code": "x=1"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if result.Description != "review prompt" {
		t.Fatalf("description = %v", result.Description)
	}
	if len(result.Messages) != 1 || result.Messages[0].Content.Text != "Review: x=1" {
		t.Fatalf("unexpected messages: %+v", result.Messages)
	}
}

func TestMemoryPromptProvider_GetNotFound(t *testing.T) {
	p := NewMemoryPromptProvider()
	_, err := p.GetPrompt(context.Background(), "nope", nil)
	if err != ErrPromptNotFound {
		t.Fatalf("expected ErrPromptNotFound, got %v", err)
	}
}

// ─── Runtime integration ─────────────────────────────────────────────────────

func TestRuntime_WithResourcesAndPrompts(t *testing.T) {
	rt := NewRuntime()
	resources := NewMemoryResourceProvider()
	_ = resources.Register(Resource{URI: "r://1", Name: "one"}, []ResourceContent{{URI: "r://1", Text: "data"}})
	rt.WithResources(resources)

	prompts := NewMemoryPromptProvider()
	_ = prompts.Register(Prompt{Name: "p1"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{Messages: []PromptMessage{{Role: "user", Content: PromptContent{Type: "text", Text: "hi"}}}}, nil
	})
	rt.WithPrompts(prompts)

	ctx := context.Background()

	res, err := rt.ListResources(ctx)
	if err != nil || len(res) != 1 {
		t.Fatalf("ListResources = %v, %v", res, err)
	}

	content, err := rt.ReadResource(ctx, "r://1")
	if err != nil || len(content) != 1 || content[0].Text != "data" {
		t.Fatalf("ReadResource = %v, %v", content, err)
	}

	templates, err := rt.ListResourceTemplates(ctx)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	_ = templates

	promptsList, err := rt.ListPrompts(ctx)
	if err != nil || len(promptsList) != 1 {
		t.Fatalf("ListPrompts = %v, %v", promptsList, err)
	}

	promptResult, err := rt.GetPrompt(ctx, "p1", nil)
	if err != nil || len(promptResult.Messages) != 1 {
		t.Fatalf("GetPrompt = %v, %v", promptResult, err)
	}
}

func TestRuntime_NilProvidersReturnNil(t *testing.T) {
	rt := NewRuntime()
	ctx := context.Background()

	res, err := rt.ListResources(ctx)
	if err != nil || res != nil {
		t.Fatalf("ListResources should return nil when no provider: %v, %v", res, err)
	}

	_, err = rt.ReadResource(ctx, "any")
	if err != ErrResourceNotFound {
		t.Fatalf("ReadResource should return ErrResourceNotFound: %v", err)
	}

	prompts, err := rt.ListPrompts(ctx)
	if err != nil || prompts != nil {
		t.Fatalf("ListPrompts should return nil when no provider: %v, %v", prompts, err)
	}

	_, err = rt.GetPrompt(ctx, "any", nil)
	if err != ErrPromptNotFound {
		t.Fatalf("GetPrompt should return ErrPromptNotFound: %v", err)
	}
}
