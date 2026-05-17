package interaction

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemorySessionStore is an in-memory SessionStore for tests and small previews.
type MemorySessionStore struct {
	mu       sync.RWMutex
	next     uint64
	sessions map[SessionID]Session
	now      func() time.Time
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[SessionID]Session),
		now:      time.Now,
	}
}

func (s *MemorySessionStore) Create(ctx context.Context, subject string, metadata map[string]string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.next++
	now := s.now()
	session := Session{
		ID:        SessionID(fmt.Sprintf("session-%d", s.next)),
		Subject:   subject,
		Metadata:  cloneStringMap(metadata),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *MemorySessionStore) Get(ctx context.Context, id SessionID) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return copySession(session), nil
}

func (s *MemorySessionStore) Close(ctx context.Context, id SessionID) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if session.Closed() {
		return copySession(session), nil
	}
	now := s.now()
	session.UpdatedAt = now
	session.ClosedAt = now
	s.sessions[id] = session
	return copySession(session), nil
}

// MemoryEventSink stores events in memory by session id.
type MemoryEventSink struct {
	mu     sync.RWMutex
	events map[SessionID][]Event
	now    func() time.Time
}

func NewMemoryEventSink() *MemoryEventSink {
	return &MemoryEventSink{
		events: make(map[SessionID][]Event),
		now:    time.Now,
	}
}

func (s *MemoryEventSink) Emit(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.At.IsZero() {
		event.At = s.now()
	}
	event.Metadata = cloneStringMap(event.Metadata)
	s.events[event.SessionID] = append(s.events[event.SessionID], event)
	return nil
}

func (s *MemoryEventSink) List(ctx context.Context, id SessionID) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[id]
	out := make([]Event, len(events))
	for i, event := range events {
		out[i] = copyEvent(event)
	}
	return out, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copySession(session Session) Session {
	session.Metadata = cloneStringMap(session.Metadata)
	return session
}

func copyEvent(event Event) Event {
	event.Metadata = cloneStringMap(event.Metadata)
	return event
}
