package interaction

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAuthorizationHookAllowsAndDeniesToolCalls(t *testing.T) {
	rt := NewRuntime().WithHooks(AuthorizationHook{
		Authorizer: AuthorizerFunc(func(ctx context.Context, session Session, call ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{Allowed: call.Name == "allowed", Reason: "blocked tool"}, nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{ToolName: "allowed", Fn: echoTool}); err != nil {
		t.Fatalf("Register allowed: %v", err)
	}
	if err := rt.RegisterTool(ToolFunc{ToolName: "blocked", Fn: echoTool}); err != nil {
		t.Fatalf("Register blocked: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if _, err := rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "allowed"}); err != nil {
		t.Fatalf("Call allowed: %v", err)
	}
	_, err = rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "blocked"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Call blocked err = %v, want ErrUnauthorized", err)
	}

	events, err := rt.Events.List(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("List events: %v", err)
	}
	got := []EventType{events[0].Type, events[1].Type, events[2].Type}
	want := []EventType{EventSessionStarted, EventToolCall, EventToolResult}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want denied call to be rejected before event emission", got)
	}
}

func TestAuditHookRecordsBeforeAndAfterToolCalls(t *testing.T) {
	var records []AuditRecord
	rt := NewRuntime().WithHooks(AuditHook{
		Sink: AuditSinkFunc(func(ctx context.Context, record AuditRecord) error {
			records = append(records, record)
			return nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{ToolName: "echo", Fn: echoTool}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	_, err = rt.CallTool(context.Background(), ToolCall{
		SessionID: session.ID,
		Name:      "echo",
		Input:     "hello",
		Metadata:  map[string]string{"trace": "abc"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("audit records length = %d, want 2", len(records))
	}
	if records[0].Phase != "before" || records[0].Tool != "echo" || !records[0].Allowed {
		t.Fatalf("unexpected before record: %+v", records[0])
	}
	if records[1].Phase != "after" || records[1].Tool != "echo" || !records[1].Allowed {
		t.Fatalf("unexpected after record: %+v", records[1])
	}
	if records[0].Metadata["trace"] != "abc" {
		t.Fatalf("metadata = %+v, want trace=abc", records[0].Metadata)
	}
}

func TestAuditHookRecordsToolErrors(t *testing.T) {
	var records []AuditRecord
	rt := NewRuntime().WithHooks(AuditHook{
		Sink: AuditSinkFunc(func(ctx context.Context, record AuditRecord) error {
			records = append(records, record)
			return nil
		}),
	})
	if err := rt.RegisterTool(ToolFunc{
		ToolName: "fail",
		Fn: func(context.Context, ToolCall) (ToolResult, error) {
			return ToolResult{}, errors.New("boom")
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := rt.StartSession(context.Background(), "agent", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	_, _ = rt.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "fail"})
	if len(records) != 2 {
		t.Fatalf("audit records length = %d, want 2", len(records))
	}
	if records[1].Allowed || records[1].Error != "boom" {
		t.Fatalf("unexpected error audit record: %+v", records[1])
	}
}

func echoTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	return ToolResult{Output: call.Input, Metadata: map[string]string{"tool": call.Name}}, nil
}

// --- AuthorizationError unit tests ---

func TestAuthorizationError_Error_WithReason(t *testing.T) {
	err := AuthorizationError{Reason: "insufficient privileges"}
	got := err.Error()
	want := "interaction: unauthorized: insufficient privileges"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAuthorizationError_Error_EmptyReason(t *testing.T) {
	err := AuthorizationError{}
	got := err.Error()
	want := ErrUnauthorized.Error()
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAuthorizationError_Unwrap(t *testing.T) {
	err := AuthorizationError{Reason: "test"}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal("AuthorizationError should unwrap to ErrUnauthorized")
	}
}

// --- AuthorizerFunc unit tests ---

func TestAuthorizerFunc_Nil_AllowsByDefault(t *testing.T) {
	var f AuthorizerFunc // nil
	decision, err := f.AuthorizeToolCall(context.Background(), Session{}, ToolCall{})
	if err != nil {
		t.Fatalf("nil AuthorizerFunc should not error, got %v", err)
	}
	if !decision.Allowed {
		t.Fatal("nil AuthorizerFunc should allow by default")
	}
}

func TestAuthorizerFunc_Custom(t *testing.T) {
	f := AuthorizerFunc(func(_ context.Context, _ Session, call ToolCall) (AuthorizationDecision, error) {
		if call.Name == "admin" {
			return AuthorizationDecision{Allowed: false, Reason: "admin only"}, nil
		}
		return AuthorizationDecision{Allowed: true}, nil
	})

	dec, err := f.AuthorizeToolCall(context.Background(), Session{}, ToolCall{Name: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Allowed {
		t.Fatal("admin tool should be denied")
	}

	dec, err = f.AuthorizeToolCall(context.Background(), Session{}, ToolCall{Name: "public"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dec.Allowed {
		t.Fatal("public tool should be allowed")
	}
}

// --- AuthorizationHook unit tests ---

func TestAuthorizationHook_NilAuthorizer(t *testing.T) {
	h := AuthorizationHook{}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if err != nil {
		t.Fatalf("nil Authorizer should return nil, got %v", err)
	}
}

func TestAuthorizationHook_Denied_NoReason_ReturnsErrUnauthorized(t *testing.T) {
	h := AuthorizationHook{
		Authorizer: AuthorizerFunc(func(_ context.Context, _ Session, _ ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{Allowed: false}, nil
		}),
	}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for empty reason, got %v", err)
	}
}

func TestAuthorizationHook_Denied_WithReason(t *testing.T) {
	h := AuthorizationHook{
		Authorizer: AuthorizerFunc(func(_ context.Context, _ Session, _ ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{Allowed: false, Reason: "forbidden"}, nil
		}),
	}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if err == nil {
		t.Fatal("denied decision should return error")
	}
	var authErr AuthorizationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthorizationError, got %T: %v", err, err)
	}
	if authErr.Reason != "forbidden" {
		t.Fatalf("expected reason 'forbidden', got %q", authErr.Reason)
	}
}

func TestAuthorizationHook_AuthorizerError(t *testing.T) {
	authErr := errors.New("auth service down")
	h := AuthorizationHook{
		Authorizer: AuthorizerFunc(func(_ context.Context, _ Session, _ ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{}, authErr
		}),
	}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if !errors.Is(err, authErr) {
		t.Fatalf("expected authorizer error, got %v", err)
	}
}

func TestAuthorizationHook_AfterToolCall_ReturnsNil(t *testing.T) {
	h := AuthorizationHook{}
	err := h.AfterToolCall(context.Background(), Session{}, ToolCall{}, ToolResult{}, nil)
	if err != nil {
		t.Fatalf("AfterToolCall should return nil, got %v", err)
	}
}

// --- AuditSinkFunc unit tests ---

func TestAuditSinkFunc_Nil(t *testing.T) {
	var f AuditSinkFunc // nil
	err := f.RecordAudit(context.Background(), AuditRecord{})
	if err != nil {
		t.Fatalf("nil AuditSinkFunc should return nil, got %v", err)
	}
}

func TestAuditSinkFunc_Custom(t *testing.T) {
	var captured AuditRecord
	f := AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
		captured = record
		return nil
	})
	err := f.RecordAudit(context.Background(), AuditRecord{Tool: "echo", Phase: "before"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Tool != "echo" {
		t.Fatalf("expected tool 'echo', got %q", captured.Tool)
	}
}

// --- AuditHook unit tests ---

func TestAuditHook_NilSink(t *testing.T) {
	h := AuditHook{}
	err := h.BeforeToolCall(context.Background(), Session{}, ToolCall{})
	if err != nil {
		t.Fatalf("nil Sink should return nil, got %v", err)
	}
	err = h.AfterToolCall(context.Background(), Session{}, ToolCall{}, ToolResult{}, nil)
	if err != nil {
		t.Fatalf("nil Sink should return nil, got %v", err)
	}
}

func TestAuditHook_CustomNow(t *testing.T) {
	fixedTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	var captured AuditRecord
	h := AuditHook{
		Sink: AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
			captured = record
			return nil
		}),
		Now: func() time.Time { return fixedTime },
	}
	h.BeforeToolCall(context.Background(), Session{ID: "s1"}, ToolCall{Name: "test"})
	if !captured.At.Equal(fixedTime) {
		t.Fatalf("expected At=%v, got %v", fixedTime, captured.At)
	}
}

func TestAuditHook_MetadataCloned(t *testing.T) {
	var captured AuditRecord
	h := AuditHook{
		Sink: AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
			captured = record
			return nil
		}),
	}
	original := map[string]string{"key": "original"}
	h.BeforeToolCall(context.Background(), Session{}, ToolCall{Name: "test", Metadata: original})

	original["key"] = "mutated"
	if captured.Metadata["key"] != "original" {
		t.Fatal("metadata should have been cloned, not shared")
	}
}

func TestAuditHook_AfterToolCall_WithError(t *testing.T) {
	var captured AuditRecord
	h := AuditHook{
		Sink: AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
			captured = record
			return nil
		}),
	}
	toolErr := errors.New("tool broke")
	h.AfterToolCall(context.Background(), Session{}, ToolCall{Name: "x"}, ToolResult{}, toolErr)
	if captured.Allowed {
		t.Fatal("with error means Allowed=false")
	}
	if captured.Error != "tool broke" {
		t.Fatalf("expected error 'tool broke', got %q", captured.Error)
	}
}

func TestAuditHook_AfterToolCall_Success(t *testing.T) {
	var captured AuditRecord
	h := AuditHook{
		Sink: AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
			captured = record
			return nil
		}),
	}
	h.AfterToolCall(context.Background(), Session{}, ToolCall{Name: "x"}, ToolResult{}, nil)
	if !captured.Allowed {
		t.Fatal("no error means Allowed=true")
	}
	if captured.Error != "" {
		t.Fatalf("expected empty error, got %q", captured.Error)
	}
}
