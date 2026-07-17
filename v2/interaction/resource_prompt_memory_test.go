package interaction

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- MemoryResourceProvider ---

func TestMemoryResourceProvider_RegisterText(t *testing.T) {
	p := NewMemoryResourceProvider()
	err := p.RegisterText("file:///readme", "readme", "Hello, World!")
	if err != nil {
		t.Fatalf("RegisterText: %v", err)
	}

	// Verify resource was registered
	resources, err := p.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].URI != "file:///readme" {
		t.Fatalf("expected URI 'file:///readme', got %q", resources[0].URI)
	}
	if resources[0].Name != "readme" {
		t.Fatalf("expected name 'readme', got %q", resources[0].Name)
	}
	if resources[0].MIMEType != "text/plain" {
		t.Fatalf("expected MIMEType 'text/plain', got %q", resources[0].MIMEType)
	}

	// Verify content
	content, err := p.ReadResource(context.Background(), "file:///readme")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content, got %d", len(content))
	}
	if content[0].Text != "Hello, World!" {
		t.Fatalf("expected text 'Hello, World!', got %q", content[0].Text)
	}
	if content[0].MIMEType != "text/plain" {
		t.Fatalf("expected content MIMEType 'text/plain', got %q", content[0].MIMEType)
	}
}

func TestMemoryResourceProvider_RegisterText_Duplicate(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.RegisterText("file:///readme", "readme", "v1")
	err := p.RegisterText("file:///readme", "readme", "v2")
	if !errors.Is(err, ErrResourceExists) {
		t.Fatalf("expected ErrResourceExists, got %v", err)
	}
}

func TestMemoryResourceProvider_Register_EmptyURI(t *testing.T) {
	p := NewMemoryResourceProvider()
	err := p.Register(Resource{Name: "test"}, nil)
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
}

func TestMemoryResourceProvider_Register_EmptyName(t *testing.T) {
	p := NewMemoryResourceProvider()
	err := p.Register(Resource{URI: "file:///test"}, nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestMemoryResourceProvider_SetTemplates_And_ListResourceTemplates(t *testing.T) {
	p := NewMemoryResourceProvider()
	templates := []ResourceTemplate{
		{URITemplate: "file:///{name}", Name: "dynamic", MIMEType: "text/plain"},
		{URITemplate: "db:///{table}/{id}", Name: "database"},
	}
	p.SetTemplates(templates)

	got, err := p.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(got))
	}
	if got[0].URITemplate != "file:///{name}" {
		t.Fatalf("unexpected template URI: %q", got[0].URITemplate)
	}
	if got[1].Name != "database" {
		t.Fatalf("unexpected template name: %q", got[1].Name)
	}
}

func TestMemoryResourceProvider_SetTemplates_ClonesInput(t *testing.T) {
	p := NewMemoryResourceProvider()
	original := []ResourceTemplate{{Name: "orig"}}
	p.SetTemplates(original)

	// Mutate original — should not affect stored templates
	original[0].Name = "mutated"
	got, _ := p.ListResourceTemplates(context.Background())
	if got[0].Name != "orig" {
		t.Fatal("SetTemplates should clone input slice")
	}
}

func TestMemoryResourceProvider_ListResourceTemplates_CancelledContext(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.SetTemplates([]ResourceTemplate{{Name: "x"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.ListResourceTemplates(ctx)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestMemoryResourceProvider_ListResources_SortedByURI(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.RegisterText("file:///c", "c", "third")
	p.RegisterText("file:///a", "a", "first")
	p.RegisterText("file:///b", "b", "second")

	resources, err := p.ListResources(context.Background())
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(resources))
	}
	if resources[0].URI != "file:///a" {
		t.Fatalf("expected first URI 'file:///a', got %q", resources[0].URI)
	}
	if resources[1].URI != "file:///b" {
		t.Fatalf("expected second URI 'file:///b', got %q", resources[1].URI)
	}
	if resources[2].URI != "file:///c" {
		t.Fatalf("expected third URI 'file:///c', got %q", resources[2].URI)
	}
}

func TestMemoryResourceProvider_ReadResource_NotFound(t *testing.T) {
	p := NewMemoryResourceProvider()
	_, err := p.ReadResource(context.Background(), "file:///nonexistent")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestMemoryResourceProvider_ReadResource_CancelledContext(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.RegisterText("file:///test", "test", "data")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.ReadResource(ctx, "file:///test")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestMemoryResourceProvider_ReadResource_ReturnsCopy(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.RegisterText("file:///test", "test", "original")

	content1, _ := p.ReadResource(context.Background(), "file:///test")
	content1[0].Text = "mutated"

	content2, _ := p.ReadResource(context.Background(), "file:///test")
	if content2[0].Text != "original" {
		t.Fatal("ReadResource should return copies, not shared data")
	}
}

func TestMemoryResourceProvider_ListResources_MetadataCloned(t *testing.T) {
	p := NewMemoryResourceProvider()
	p.Register(Resource{URI: "file:///test", Name: "test", Metadata: map[string]string{"key": "val"}}, nil)

	resources, _ := p.ListResources(context.Background())
	resources[0].Metadata["key"] = "mutated"

	resources2, _ := p.ListResources(context.Background())
	if resources2[0].Metadata["key"] != "val" {
		t.Fatal("ListResources metadata should be cloned")
	}
}

func TestMemoryResourceProvider_RegisterClonesMutableInput(t *testing.T) {
	p := NewMemoryResourceProvider()
	metadata := map[string]string{"owner": "original"}
	blob := []byte("original")
	contents := []ResourceContent{{URI: "file:///test", Blob: blob}}
	if err := p.Register(Resource{URI: "file:///test", Name: "test", Metadata: metadata}, contents); err != nil {
		t.Fatalf("Register: %v", err)
	}

	metadata["owner"] = "mutated"
	blob[0] = 'X'
	contents[0].URI = "mutated"

	resources, _ := p.ListResources(context.Background())
	if resources[0].Metadata["owner"] != "original" {
		t.Fatal("Register should clone resource metadata")
	}
	got, _ := p.ReadResource(context.Background(), "file:///test")
	if got[0].URI != "file:///test" || string(got[0].Blob) != "original" {
		t.Fatalf("Register should clone resource content, got %#v", got[0])
	}
	got[0].Blob[0] = 'Y'
	again, _ := p.ReadResource(context.Background(), "file:///test")
	if string(again[0].Blob) != "original" {
		t.Fatal("ReadResource should deep-clone blobs")
	}
}

// --- MemoryPromptProvider ---

func TestMemoryPromptProvider_CompleteArgument_ExistingPrompt(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "greet"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{}, nil
	})

	result, err := p.CompleteArgument(context.Background(), "greet", "name", "he")
	if err != nil {
		t.Fatalf("CompleteArgument: %v", err)
	}
	if len(result.Values) != 0 {
		t.Fatalf("expected empty values, got %v", result.Values)
	}
	if result.Total != 0 {
		t.Fatalf("expected Total=0, got %d", result.Total)
	}
	if result.HasMore {
		t.Fatal("expected HasMore=false")
	}
}

func TestMemoryPromptProvider_CompleteArgument_NonexistentPrompt(t *testing.T) {
	p := NewMemoryPromptProvider()
	_, err := p.CompleteArgument(context.Background(), "nonexistent", "arg", "val")
	if !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("expected ErrPromptNotFound, got %v", err)
	}
}

func TestMemoryPromptProvider_CompleteArgument_CancelledContext(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "test"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.CompleteArgument(ctx, "test", "arg", "val")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestMemoryPromptProvider_Register_EmptyName(t *testing.T) {
	p := NewMemoryPromptProvider()
	err := p.Register(Prompt{}, nil)
	if err == nil {
		t.Fatal("expected error for empty prompt name")
	}
}

func TestMemoryPromptProvider_Register_Duplicate(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "test"}, nil)
	err := p.Register(Prompt{Name: "test"}, nil)
	if !errors.Is(err, ErrPromptExists) {
		t.Fatalf("expected ErrPromptExists, got %v", err)
	}
}

func TestMemoryPromptProvider_ListPrompts_SortedByName(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "zebra"}, nil)
	p.Register(Prompt{Name: "alpha"}, nil)
	p.Register(Prompt{Name: "middle"}, nil)

	prompts, err := p.ListPrompts(context.Background())
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(prompts))
	}
	if prompts[0].Name != "alpha" {
		t.Fatalf("expected first name 'alpha', got %q", prompts[0].Name)
	}
	if prompts[1].Name != "middle" {
		t.Fatalf("expected second name 'middle', got %q", prompts[1].Name)
	}
	if prompts[2].Name != "zebra" {
		t.Fatalf("expected third name 'zebra', got %q", prompts[2].Name)
	}
}

func TestMemoryPromptProvider_GetPrompt_NotFound(t *testing.T) {
	p := NewMemoryPromptProvider()
	_, err := p.GetPrompt(context.Background(), "nonexistent", nil)
	if !errors.Is(err, ErrPromptNotFound) {
		t.Fatalf("expected ErrPromptNotFound, got %v", err)
	}
}

func TestMemoryPromptProvider_GetPrompt_NilRender(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "test"}, nil)
	_, err := p.GetPrompt(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error when render is nil")
	}
}

func TestMemoryPromptProvider_GetPrompt_Success(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "greet"}, func(args map[string]string) (PromptResult, error) {
		return PromptResult{
			Description: "A greeting",
			Messages: []PromptMessage{
				{Role: "user", Content: PromptContent{Type: "text", Text: "Hello " + args["name"]}},
			},
		}, nil
	})

	result, err := p.GetPrompt(context.Background(), "greet", map[string]string{"name": "Alice"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if result.Description != "A greeting" {
		t.Fatalf("expected description 'A greeting', got %q", result.Description)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Content.Text != "Hello Alice" {
		t.Fatalf("expected 'Hello Alice', got %q", result.Messages[0].Content.Text)
	}
}

func TestMemoryPromptProvider_ListPrompts_CancelledContext(t *testing.T) {
	p := NewMemoryPromptProvider()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.ListPrompts(ctx)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestMemoryPromptProvider_GetPrompt_CancelledContext(t *testing.T) {
	p := NewMemoryPromptProvider()
	p.Register(Prompt{Name: "test"}, func(_ map[string]string) (PromptResult, error) {
		return PromptResult{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.GetPrompt(ctx, "test", nil)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestMemoryPromptProvider_ClonesPromptAndRenderArguments(t *testing.T) {
	p := NewMemoryPromptProvider()
	prompt := Prompt{Name: "test", Arguments: []PromptArgument{{Name: "value"}}}
	if err := p.Register(prompt, func(args map[string]string) (PromptResult, error) {
		args["value"] = "mutated by render"
		return PromptResult{}, nil
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	prompt.Arguments[0].Name = "mutated"

	prompts, _ := p.ListPrompts(context.Background())
	if prompts[0].Arguments[0].Name != "value" {
		t.Fatal("Register should clone prompt arguments")
	}
	prompts[0].Arguments[0].Name = "mutated again"
	again, _ := p.ListPrompts(context.Background())
	if again[0].Arguments[0].Name != "value" {
		t.Fatal("ListPrompts should return cloned prompt arguments")
	}

	args := map[string]string{"value": "original"}
	if _, err := p.GetPrompt(context.Background(), "test", args); err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if args["value"] != "original" {
		t.Fatal("GetPrompt should isolate caller arguments from render callbacks")
	}
}

func TestMemoryPromptProvider_RenderRunsOutsideLock(t *testing.T) {
	p := NewMemoryPromptProvider()
	if err := p.Register(Prompt{Name: "test"}, func(map[string]string) (PromptResult, error) {
		_, err := p.ListPrompts(context.Background())
		return PromptResult{}, err
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := p.GetPrompt(context.Background(), "test", nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("render callback deadlocked while re-entering provider")
	}
}
