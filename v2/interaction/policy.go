package interaction

import (
	"context"
	"time"
)

// AuthorizationDecision explains whether a tool call is allowed.
type AuthorizationDecision struct {
	Allowed  bool
	Reason   string
	Metadata map[string]string
}

// Authorizer decides whether a session can call a tool.
type Authorizer interface {
	AuthorizeToolCall(ctx context.Context, session Session, call ToolCall) (AuthorizationDecision, error)
}

// AuthorizerFunc adapts a function into an Authorizer.
type AuthorizerFunc func(context.Context, Session, ToolCall) (AuthorizationDecision, error)

func (f AuthorizerFunc) AuthorizeToolCall(ctx context.Context, session Session, call ToolCall) (AuthorizationDecision, error) {
	if f == nil {
		return AuthorizationDecision{Allowed: true}, nil
	}
	return f(ctx, session, call)
}

// AuthorizationHook rejects tool calls denied by an Authorizer.
type AuthorizationHook struct {
	Authorizer Authorizer
}

func (h AuthorizationHook) BeforeToolCall(ctx context.Context, session Session, call ToolCall) error {
	if h.Authorizer == nil {
		return nil
	}
	decision, err := h.Authorizer.AuthorizeToolCall(ctx, session, call)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		if decision.Reason == "" {
			return ErrUnauthorized
		}
		return AuthorizationError{Reason: decision.Reason}
	}
	return nil
}

func (h AuthorizationHook) AfterToolCall(context.Context, Session, ToolCall, ToolResult, error) error {
	return nil
}

// AuthorizationError reports a rejected tool call.
type AuthorizationError struct {
	Reason string
}

func (e AuthorizationError) Error() string {
	if e.Reason == "" {
		return ErrUnauthorized.Error()
	}
	return ErrUnauthorized.Error() + ": " + e.Reason
}

func (e AuthorizationError) Unwrap() error {
	return ErrUnauthorized
}

// AuditRecord captures one interaction runtime decision or result.
type AuditRecord struct {
	SessionID SessionID
	Subject   string
	Tool      string
	Phase     string
	Allowed   bool
	Error     string
	Metadata  map[string]string
	At        time.Time
}

// AuditSink records interaction runtime audit events.
type AuditSink interface {
	RecordAudit(ctx context.Context, record AuditRecord) error
}

// AuditSinkFunc adapts a function into an AuditSink.
type AuditSinkFunc func(context.Context, AuditRecord) error

func (f AuditSinkFunc) RecordAudit(ctx context.Context, record AuditRecord) error {
	if f == nil {
		return nil
	}
	return f(ctx, record)
}

// AuditHook records before and after tool call audit events.
type AuditHook struct {
	Sink AuditSink
	Now  func() time.Time
}

func (h AuditHook) BeforeToolCall(ctx context.Context, session Session, call ToolCall) error {
	return h.record(ctx, AuditRecord{
		SessionID: session.ID,
		Subject:   session.Subject,
		Tool:      call.Name,
		Phase:     "before",
		Allowed:   true,
		Metadata:  call.Metadata,
	})
}

func (h AuditHook) AfterToolCall(ctx context.Context, session Session, call ToolCall, result ToolResult, err error) error {
	record := AuditRecord{
		SessionID: session.ID,
		Subject:   session.Subject,
		Tool:      call.Name,
		Phase:     "after",
		Allowed:   err == nil,
		Metadata:  result.Metadata,
	}
	if err != nil {
		record.Error = err.Error()
	}
	return h.record(ctx, record)
}

func (h AuditHook) record(ctx context.Context, record AuditRecord) error {
	if h.Sink == nil {
		return nil
	}
	if record.At.IsZero() {
		now := h.Now
		if now == nil {
			now = time.Now
		}
		record.At = now()
	}
	record.Metadata = cloneStringMap(record.Metadata)
	return h.Sink.RecordAudit(ctx, record)
}
