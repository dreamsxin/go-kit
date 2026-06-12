package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// SamplingMessage is a single message in a sampling conversation.
type SamplingMessage struct {
	Role    string          `json:"role"`    // "user" | "assistant"
	Content SamplingContent `json:"content"` // text, image, or audio
}

// SamplingContent carries the body of a sampling message.
type SamplingContent struct {
	Type     string `json:"type"`               // "text" | "image" | "audio"
	Text     string `json:"text,omitempty"`     // for text
	Data     string `json:"data,omitempty"`     // base64 for image/audio
	MIMEType string `json:"mimeType,omitempty"` // for image/audio
}

// ModelHint suggests a model name substring to match.
type ModelHint struct {
	Name string `json:"name,omitempty"`
}

// ModelPreferences expresses the server's model selection priorities.
type ModelPreferences struct {
	Hints                []ModelHint `json:"hints,omitempty"`
	CostPriority         float64     `json:"costPriority,omitempty"`
	SpeedPriority        float64     `json:"speedPriority,omitempty"`
	IntelligencePriority float64     `json:"intelligencePriority,omitempty"`
}

// CreateMessageRequest is the params of a sampling/createMessage request.
type CreateMessageRequest struct {
	Messages          []SamplingMessage `json:"messages"`
	ModelPreferences  *ModelPreferences `json:"modelPreferences,omitempty"`
	SystemPrompt      string            `json:"systemPrompt,omitempty"`
	IncludeContext    string            `json:"includeContext,omitempty"` // "none"|"thisServer"|"allServers"
	Temperature       float64           `json:"temperature,omitempty"`
	MaxTokens         int               `json:"maxTokens"`
	StopSequences     []string          `json:"stopSequences,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
}

// CreateMessageResult is the result of a sampling/createMessage request.
type CreateMessageResult struct {
	Role       string          `json:"role"`
	Content    SamplingContent `json:"content"`
	Model      string          `json:"model"`
	StopReason string          `json:"stopReason,omitempty"` // "endTurn"|"stopSequence"|"maxTokens"
}

// Sampler sends sampling/createMessage requests to the connected MCP client
// and blocks until the client responds.
type Sampler struct {
	mu       sync.Mutex
	nextID   atomic.Int64
	sessions map[string]*pendingTracker // keyed by MCP session ID
}

type pendingTracker struct {
	mu      sync.Mutex
	pending map[string]chan CreateMessageResult // keyed by request ID string
}

// NewSampler creates a Sampler.
func NewSampler() *Sampler {
	return &Sampler{
		sessions: make(map[string]*pendingTracker),
	}
}

// RegisterSession adds a session's pending-request tracker.
func (s *Sampler) RegisterSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = &pendingTracker{pending: make(map[string]chan CreateMessageResult)}
}

// UnregisterSession removes a session's tracker.
func (s *Sampler) UnregisterSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.sessions[sessionID]; ok {
		t.mu.Lock()
		for id, ch := range t.pending {
			close(ch)
			delete(t.pending, id)
		}
		t.mu.Unlock()
		delete(s.sessions, sessionID)
	}
}

// CreateMessage sends a sampling/createMessage request to the client connected
// on the given session. The sendFn is called to write the JSON-RPC request onto
// the SSE stream. The method then blocks until the client responds or the
// context is cancelled.
func (s *Sampler) CreateMessage(ctx context.Context, sessionID string, req CreateMessageRequest, sendFn func(json.RawMessage) error) (CreateMessageResult, error) {
	id := s.nextID.Add(1)
	idStr := fmt.Sprintf("sampling-%d", id)

	tracker := s.getTracker(sessionID)
	if tracker == nil {
		return CreateMessageResult{}, fmt.Errorf("mcp: no active session %q", sessionID)
	}

	ch := make(chan CreateMessageResult, 1)
	tracker.mu.Lock()
	tracker.pending[idStr] = ch
	tracker.mu.Unlock()

	defer func() {
		tracker.mu.Lock()
		delete(tracker.pending, idStr)
		tracker.mu.Unlock()
	}()

	// Build the JSON-RPC request.
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      idStr,
		"method":  "sampling/createMessage",
		"params":  req,
	}
	raw, err := json.Marshal(rpcReq)
	if err != nil {
		return CreateMessageResult{}, fmt.Errorf("mcp: marshal sampling request: %w", err)
	}
	if err := sendFn(raw); err != nil {
		return CreateMessageResult{}, fmt.Errorf("mcp: send sampling request: %w", err)
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return CreateMessageResult{}, fmt.Errorf("mcp: sampling session closed")
		}
		return result, nil
	case <-ctx.Done():
		return CreateMessageResult{}, ctx.Err()
	}
}

// DeliverResponse delivers a client response to a pending sampling request.
// Returns false if the session or request ID is not found.
func (s *Sampler) DeliverResponse(sessionID string, idStr string, result CreateMessageResult) (ok bool) {
	tracker := s.getTracker(sessionID)
	if tracker == nil {
		return false
	}
	tracker.mu.Lock()
	ch, found := tracker.pending[idStr]
	tracker.mu.Unlock()
	if !found {
		return false
	}
	// Guard against a closed channel: UnregisterSession may close ch
	// between the lock release above and this send.
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	ch <- result
	return true
}

func (s *Sampler) getTracker(sessionID string) *pendingTracker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[sessionID]
}
