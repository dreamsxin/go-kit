package interaction

import (
	"context"
	"errors"
	"testing"
)

// A denial that never reaches the audit sink is a hole in the audit trail: the
// only record used to be the "before" one, whose Allowed is always true.
func TestCallToolAuditsARejectedCall(t *testing.T) {
	var records []AuditRecord
	sink := AuditSinkFunc(func(_ context.Context, record AuditRecord) error {
		records = append(records, record)
		return nil
	})

	runtime := NewRuntime().WithHooks(
		AuditHook{Sink: sink},
		AuthorizationHook{Authorizer: AuthorizerFunc(func(context.Context, Session, ToolCall) (AuthorizationDecision, error) {
			return AuthorizationDecision{Reason: "not your tool"}, nil
		})},
	)
	tool := ToolFunc{ToolName: "t", Fn: func(context.Context, ToolCall) (ToolResult, error) {
		t.Error("the tool must not run")
		return ToolResult{}, nil
	}}
	if err := runtime.RegisterTool(tool); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	session, err := runtime.StartSession(context.Background(), "alice", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	_, err = runtime.CallTool(context.Background(), ToolCall{SessionID: session.ID, Name: "t"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}

	var after *AuditRecord
	for i := range records {
		if records[i].Phase == "after" {
			after = &records[i]
		}
	}
	if after == nil {
		t.Fatalf("no after record was written: %+v", records)
	}
	if after.Allowed {
		t.Fatal("the after record of a rejected call must not report Allowed")
	}
	if after.Error == "" {
		t.Fatal("the after record should carry the rejection")
	}
}

// A zero Runtime has no working components; reporting that beats a nil panic
// inside a request.
func TestZeroRuntimeReportsMissingComponents(t *testing.T) {
	runtime := &Runtime{}

	if _, err := runtime.StartSession(context.Background(), "alice", nil); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("StartSession err = %v, want ErrRuntimeNotConfigured", err)
	}
	if _, err := runtime.CallTool(context.Background(), ToolCall{}); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("CallTool err = %v, want ErrRuntimeNotConfigured", err)
	}
	if err := runtime.RegisterTool(nil); !errors.Is(err, ErrRuntimeNotConfigured) {
		t.Fatalf("RegisterTool err = %v, want ErrRuntimeNotConfigured", err)
	}
}

// recordingSessionStore remembers what it created so a test can look for a
// session the caller never received.
type recordingSessionStore struct {
	*MemorySessionStore
	created SessionID
}

func (s *recordingSessionStore) Create(ctx context.Context, subject string, metadata map[string]string) (Session, error) {
	session, err := s.MemorySessionStore.Create(ctx, subject, metadata)
	if err == nil {
		s.created = session.ID
	}
	return session, err
}

// A session the caller never received must not be left usable in the store.
func TestStartSessionReleasesTheSessionWhenTheEventFails(t *testing.T) {
	emitErr := errors.New("sink down")
	store := &recordingSessionStore{MemorySessionStore: NewMemorySessionStore()}
	runtime := NewRuntime().
		WithSessions(store).
		WithEvents(&failingEventSink{err: emitErr})

	if _, err := runtime.StartSession(context.Background(), "alice", nil); !errors.Is(err, emitErr) {
		t.Fatalf("err = %v, want the emit failure", err)
	}
	if store.created == "" {
		t.Fatal("the store was never asked to create a session")
	}

	session, err := store.Get(context.Background(), store.created)
	if errors.Is(err, ErrSessionNotFound) {
		return // deleted, which is the strongest outcome
	}
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !session.Closed() {
		t.Fatalf("session %s was left open", session.ID)
	}
}
