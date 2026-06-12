package interaction

import (
	"context"
	"fmt"
)

// Runtime coordinates sessions, events, tools, hooks, resources, and prompts.
type Runtime struct {
	Sessions  SessionStore
	Events    EventSink
	Tools     ToolRegistry
	Hooks     []Hook
	Resources ResourceProvider
	Prompts   PromptProvider

	// OnEmitError is called when event emission fails during tool calls.
	// If nil, emit errors are silently discarded.
	OnEmitError func(error)
}

// NewRuntime returns a Runtime with default in-memory components for sessions,
// events, and tools. Use the With* chaining methods or assign fields directly
// to override the defaults.
//
//	rt := interaction.NewRuntime().
//	    WithHooks(myHook).
//	    WithResources(myResources)
func NewRuntime() *Runtime {
	return &Runtime{
		Sessions: NewMemorySessionStore(),
		Events:   NewMemoryEventSink(),
		Tools:    NewMemoryToolRegistry(),
	}
}

// WithSessions overrides the SessionStore and returns the runtime for chaining.
func (r *Runtime) WithSessions(store SessionStore) *Runtime {
	r.Sessions = store
	return r
}

// WithEvents overrides the EventSink and returns the runtime for chaining.
func (r *Runtime) WithEvents(sink EventSink) *Runtime {
	r.Events = sink
	return r
}

// WithTools overrides the ToolRegistry and returns the runtime for chaining.
func (r *Runtime) WithTools(registry ToolRegistry) *Runtime {
	r.Tools = registry
	return r
}

// WithHooks appends one or more Hooks to the existing hook chain and returns
// the runtime for chaining. Unlike WithSessions, WithEvents, and WithTools
// which replace the current component, WithHooks accumulates — calling it
// multiple times adds to the chain rather than overwriting it.
func (r *Runtime) WithHooks(hooks ...Hook) *Runtime {
	r.Hooks = append(r.Hooks, hooks...)
	return r
}

// WithResources sets the resource provider and returns the runtime for chaining.
func (r *Runtime) WithResources(provider ResourceProvider) *Runtime {
	r.Resources = provider
	return r
}

// WithPrompts sets the prompt provider and returns the runtime for chaining.
func (r *Runtime) WithPrompts(provider PromptProvider) *Runtime {
	r.Prompts = provider
	return r
}

// RegisterResource lazily initializes a MemoryResourceProvider (if Resources
// is nil) and registers a resource with its content. This is a convenience
// shortcut equivalent to creating a provider externally and calling WithResources.
func (r *Runtime) RegisterResource(res Resource, content []ResourceContent) error {
	if r.Resources == nil {
		r.Resources = NewMemoryResourceProvider()
	}
	provider, ok := r.Resources.(*MemoryResourceProvider)
	if !ok {
		return fmt.Errorf("interaction: Resources is not a *MemoryResourceProvider")
	}
	return provider.Register(res, content)
}

// RegisterPrompt lazily initializes a MemoryPromptProvider (if Prompts is nil)
// and registers a prompt with its render function. This is a convenience
// shortcut equivalent to creating a provider externally and calling WithPrompts.
func (r *Runtime) RegisterPrompt(p Prompt, render func(map[string]string) (PromptResult, error)) error {
	if r.Prompts == nil {
		r.Prompts = NewMemoryPromptProvider()
	}
	provider, ok := r.Prompts.(*MemoryPromptProvider)
	if !ok {
		return fmt.Errorf("interaction: Prompts is not a *MemoryPromptProvider")
	}
	return provider.Register(p, render)
}

// StartSession creates a new session and emits a session-started event.
func (r *Runtime) StartSession(ctx context.Context, subject string, metadata map[string]string) (Session, error) {
	session, err := r.Sessions.Create(ctx, subject, metadata)
	if err != nil {
		return Session{}, err
	}
	if err := r.Events.Emit(ctx, Event{SessionID: session.ID, Type: EventSessionStarted}); err != nil {
		return Session{}, err
	}
	return session, nil
}

// EndSession closes the session and emits a session-ended event.
func (r *Runtime) EndSession(ctx context.Context, id SessionID) (Session, error) {
	session, err := r.Sessions.Close(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if err := r.Events.Emit(ctx, Event{SessionID: id, Type: EventSessionEnded}); err != nil {
		return Session{}, err
	}
	return session, nil
}

// RegisterTool registers a tool with the runtime's tool registry.
func (r *Runtime) RegisterTool(tool Tool) error {
	return r.Tools.Register(tool)
}

// ListTools returns descriptors of all registered tools.
func (r *Runtime) ListTools() []ToolDescriptor {
	lister, ok := r.Tools.(ToolLister)
	if !ok {
		return nil
	}
	return lister.List()
}

// CallTool executes a tool call, invoking hooks and emitting events.
func (r *Runtime) CallTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	session, err := r.Sessions.Get(ctx, call.SessionID)
	if err != nil {
		return ToolResult{}, err
	}
	if session.Closed() {
		return ToolResult{}, ErrSessionClosed
	}

	for _, hook := range r.Hooks {
		if err := hook.BeforeToolCall(ctx, session, call); err != nil {
			return ToolResult{}, err
		}
	}

	r.emit(ctx, Event{
		SessionID: call.SessionID,
		Type:      EventToolCall,
		Name:      call.Name,
		Payload:   call.Input,
		Metadata:  call.Metadata,
	})

	result, err := r.Tools.Call(ctx, call)
	r.emit(ctx, Event{
		SessionID: call.SessionID,
		Type:      EventToolResult,
		Name:      call.Name,
		Payload:   result.Output,
		Metadata:  result.Metadata,
	})
	if err != nil {
		r.emit(ctx, Event{
			SessionID: call.SessionID,
			Type:      EventError,
			Name:      call.Name,
			Payload:   err.Error(),
		})
	}

	for i := len(r.Hooks) - 1; i >= 0; i-- {
		if hookErr := r.Hooks[i].AfterToolCall(ctx, session, call, result, err); hookErr != nil && err == nil {
			err = hookErr
		}
	}
	return result, err
}

// emit sends an event and routes any error to OnEmitError.
func (r *Runtime) emit(ctx context.Context, ev Event) {
	if err := r.Events.Emit(ctx, ev); err != nil && r.OnEmitError != nil {
		r.OnEmitError(err)
	}
}

// ListResources returns all registered resources.
func (r *Runtime) ListResources(ctx context.Context) ([]Resource, error) {
	if r.Resources == nil {
		return nil, nil
	}
	return r.Resources.ListResources(ctx)
}

// ReadResource reads the content of a resource by URI.
func (r *Runtime) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	if r.Resources == nil {
		return nil, ErrResourceNotFound
	}
	return r.Resources.ReadResource(ctx, uri)
}

// ListResourceTemplates returns resource URI templates if the provider supports them.
func (r *Runtime) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	if r.Resources == nil {
		return nil, nil
	}
	lister, ok := r.Resources.(ResourceTemplateLister)
	if !ok {
		return nil, nil
	}
	return lister.ListResourceTemplates(ctx)
}

// ListPrompts returns all registered prompts.
func (r *Runtime) ListPrompts(ctx context.Context) ([]Prompt, error) {
	if r.Prompts == nil {
		return nil, nil
	}
	return r.Prompts.ListPrompts(ctx)
}

// GetPrompt renders a prompt by name with the given arguments.
func (r *Runtime) GetPrompt(ctx context.Context, name string, args map[string]string) (PromptResult, error) {
	if r.Prompts == nil {
		return PromptResult{}, ErrPromptNotFound
	}
	return r.Prompts.GetPrompt(ctx, name, args)
}

// CompletePromptArgument delegates to the prompt provider if it implements
// PromptCompleter, otherwise returns ErrCompletionUnsupported.
func (r *Runtime) CompletePromptArgument(ctx context.Context, promptName, argName, partialValue string) (CompletionResult, error) {
	if r.Prompts == nil {
		return CompletionResult{}, ErrCompletionUnsupported
	}
	completer, ok := r.Prompts.(PromptCompleter)
	if !ok {
		return CompletionResult{}, ErrCompletionUnsupported
	}
	return completer.CompleteArgument(ctx, promptName, argName, partialValue)
}

// HookFuncs adapts functions into a Hook.
type HookFuncs struct {
	Before func(context.Context, Session, ToolCall) error
	After  func(context.Context, Session, ToolCall, ToolResult, error) error
}

// BeforeToolCall calls h.Before if set; otherwise returns nil.
func (h HookFuncs) BeforeToolCall(ctx context.Context, session Session, call ToolCall) error {
	if h.Before == nil {
		return nil
	}
	return h.Before(ctx, session, call)
}

// AfterToolCall calls h.After if set; otherwise returns nil.
func (h HookFuncs) AfterToolCall(ctx context.Context, session Session, call ToolCall, result ToolResult, err error) error {
	if h.After == nil {
		return nil
	}
	return h.After(ctx, session, call, result, err)
}
