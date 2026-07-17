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
	postWriters  map[string]*sseWriter // keyed by writer ID
	closed       bool
	lastActivity time.Time
}

// sseWriter wraps an http.ResponseWriter for SSE streaming.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
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
	select {
	case <-sw.done:
		return fmt.Errorf("mcp: SSE stream closed")
	default:
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()
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
		postWriters:  make(map[string]*sseWriter),
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
		for _, w := range s.postWriters {
			w.close()
		}
		s.mu.Unlock()
		delete(ss.sessions, id)
	}
}

// broadcastToGET sends a JSON-RPC message to all GET SSE writers for a session.
func (ss *sseSession) broadcastToGET(data json.RawMessage) error {
	ss.mu.RLock()
	if ss.closed {
		ss.mu.RUnlock()
		return fmt.Errorf("mcp: session closed")
	}
	writers := make([]*sseWriter, 0, len(ss.getWriters))
	for _, w := range ss.getWriters {
		writers = append(writers, w)
	}
	ss.mu.RUnlock()

	for _, w := range writers {
		if err := w.writeEvent(data); err != nil {
			continue // best-effort delivery
		}
	}
	if len(writers) == 0 {
		return fmt.Errorf("mcp: no active SSE stream for session")
	}
	return nil
}

// writeToPOST sends a JSON-RPC message to one active POST SSE writer, if any.
func (ss *sseSession) writeToPOST(data json.RawMessage) (bool, error) {
	ss.mu.RLock()
	if ss.closed {
		ss.mu.RUnlock()
		return false, fmt.Errorf("mcp: session closed")
	}
	writers := make([]*sseWriter, 0, len(ss.postWriters))
	for _, w := range ss.postWriters {
		writers = append(writers, w)
	}
	ss.mu.RUnlock()

	if len(writers) == 0 {
		return false, nil
	}
	for _, w := range writers {
		if err := w.writeEvent(data); err == nil {
			return true, nil
		}
	}
	return false, fmt.Errorf("mcp: no active POST SSE stream accepted message")
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

// addPostWriter registers a POST-initiated SSE writer.
func (ss *sseSession) addPostWriter(id string, w *sseWriter) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.postWriters[id] = w
}

// removePostWriter unregisters a POST-initiated SSE writer.
func (ss *sseSession) removePostWriter(id string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.postWriters, id)
}

func newSSEWriterID(prefix, sessionID string) (string, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", err
	}
	return prefix + "-" + sessionID + "-" + id, nil
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
