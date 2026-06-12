package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// sseSession tracks one Streamable HTTP MCP session.
type sseSession struct {
	ID           string
	clientCaps   map[string]any // client capabilities from initialize
	initialized  bool
	mu           sync.RWMutex
	getWriters   map[string]*sseWriter // keyed by writer ID
	closed       bool
	lastActivity time.Time
}

// sseWriter wraps an http.ResponseWriter for SSE streaming.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{}
	once    sync.Once
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("mcp: response writer does not support http.Flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &sseWriter{w: w, flusher: flusher, done: make(chan struct{})}, nil
}

// writeEvent writes a single SSE data event with the given JSON payload.
func (sw *sseWriter) writeEvent(data json.RawMessage) error {
	if _, err := fmt.Fprintf(sw.w, "data: %s\n\n", string(data)); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

// close signals the SSE writer is done.
func (sw *sseWriter) close() {
	sw.once.Do(func() { close(sw.done) })
}

// Done returns a channel that is closed when the writer is done.
func (sw *sseWriter) Done() <-chan struct{} { return sw.done }

// sessionStore manages active SSE sessions.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sseSession
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]*sseSession)}
}

func (ss *sessionStore) create() (*sseSession, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	sess := &sseSession{
		ID:           id,
		getWriters:   make(map[string]*sseWriter),
		lastActivity: time.Now(),
	}
	ss.mu.Lock()
	ss.sessions[id] = sess
	ss.mu.Unlock()
	return sess, nil
}

func (ss *sessionStore) get(id string) (*sseSession, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.sessions[id]
	if ok {
		s.mu.Lock()
		s.lastActivity = time.Now()
		s.mu.Unlock()
	}
	return s, ok
}

func (ss *sessionStore) remove(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if s, ok := ss.sessions[id]; ok {
		s.mu.Lock()
		s.closed = true
		for _, w := range s.getWriters {
			w.close()
		}
		s.mu.Unlock()
		delete(ss.sessions, id)
	}
}

// broadcastToGET sends a JSON-RPC message to all GET SSE writers for a session.
func (ss *sseSession) broadcastToGET(data json.RawMessage) error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.closed {
		return fmt.Errorf("mcp: session closed")
	}
	for _, w := range ss.getWriters {
		if err := w.writeEvent(data); err != nil {
			continue // best-effort delivery
		}
	}
	if len(ss.getWriters) == 0 {
		return fmt.Errorf("mcp: no active SSE stream for session")
	}
	return nil
}

// addGETWriter registers an SSE writer for server-initiated messages.
func (ss *sseSession) addGETWriter(id string, w *sseWriter) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.getWriters[id] = w
}

// removeGETWriter unregisters a GET SSE writer.
func (ss *sseSession) removeGETWriter(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.getWriters, id)
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mcp: generate session id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// expiredIDs returns session IDs that have been idle longer than ttl.
func (ss *sessionStore) expiredIDs(ttl time.Duration) []string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	cutoff := time.Now().Add(-ttl)
	var ids []string
	for id, s := range ss.sessions {
		s.mu.RLock()
		if s.lastActivity.Before(cutoff) {
			ids = append(ids, id)
		}
		s.mu.RUnlock()
	}
	return ids
}
