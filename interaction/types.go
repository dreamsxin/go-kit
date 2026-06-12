// Package interaction defines runtime contracts for AI-facing interactive
// sessions, event streams, and tool-call loops.
package interaction

import (
	"context"
	"time"
)

// SessionID identifies one interactive runtime session.
type SessionID string

// EventType classifies events emitted by an interaction runtime.
type EventType string

const (
	EventSessionStarted EventType = "session.started"
	EventSessionEnded   EventType = "session.ended"
	EventToolCall       EventType = "tool.call"
	EventToolResult     EventType = "tool.result"
	EventError          EventType = "error"
	EventMessage        EventType = "message"
)

// Session tracks the lifecycle of one AI-facing interaction.
type Session struct {
	ID        SessionID
	Subject   string
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
}

// Closed reports whether the session has ended.
func (s Session) Closed() bool {
	return !s.ClosedAt.IsZero()
}

// Event is a transport-neutral interaction event.
type Event struct {
	SessionID SessionID
	Type      EventType
	Name      string
	Payload   any
	Metadata  map[string]string
	At        time.Time
}

// ToolCall is a transport-neutral tool invocation request.
type ToolCall struct {
	SessionID SessionID
	Name      string
	Input     any
	Metadata  map[string]string
}

// ToolResult is the result of executing a tool call.
type ToolResult struct {
	Output   any
	Metadata map[string]string
}

// ToolDescriptor describes one AI-callable action for discovery endpoints.
type ToolDescriptor struct {
	Name        string
	Description string
	InputSchema any
	Metadata    map[string]string
}

// Tool executes one AI-callable action.
type Tool interface {
	Name() string
	Call(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ToolDescriber lets tools expose richer discovery metadata.
type ToolDescriber interface {
	Descriptor() ToolDescriptor
}

// ToolFunc adapts a function into a Tool. Optional Description and Schema
// fields make the tool discoverable via MCP — when either is set, ToolFunc
// also satisfies ToolDescriber.
type ToolFunc struct {
	ToolName    string
	Description string // optional, advertised via tools/list
	Schema      any    // optional JSON schema for inputs
	Fn          func(context.Context, ToolCall) (ToolResult, error)
}

func (f ToolFunc) Name() string { return f.ToolName }

func (f ToolFunc) Descriptor() ToolDescriptor {
	return ToolDescriptor{
		Name:        f.ToolName,
		Description: f.Description,
		InputSchema: f.Schema,
	}
}

func (f ToolFunc) Call(ctx context.Context, call ToolCall) (ToolResult, error) {
	if f.Fn == nil {
		return ToolResult{}, ErrNilToolFunc
	}
	return f.Fn(ctx, call)
}

// SessionStore manages interaction sessions.
type SessionStore interface {
	Create(ctx context.Context, subject string, metadata map[string]string) (Session, error)
	Get(ctx context.Context, id SessionID) (Session, error)
	Close(ctx context.Context, id SessionID) (Session, error)
}

// EventSink records interaction events.
type EventSink interface {
	Emit(ctx context.Context, event Event) error
	List(ctx context.Context, id SessionID) ([]Event, error)
}

// ToolRegistry registers and executes tools.
type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, bool)
	Call(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ToolLister is implemented by registries that can list registered tools.
type ToolLister interface {
	List() []ToolDescriptor
}

// Hook observes or rejects runtime operations.
type Hook interface {
	BeforeToolCall(ctx context.Context, session Session, call ToolCall) error
	AfterToolCall(ctx context.Context, session Session, call ToolCall, result ToolResult, err error) error
}
