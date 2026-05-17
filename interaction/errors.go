package interaction

import "errors"

var (
	ErrNilToolFunc     = errors.New("interaction: nil tool func")
	ErrNilTool         = errors.New("interaction: nil tool")
	ErrEmptyToolName   = errors.New("interaction: empty tool name")
	ErrToolExists      = errors.New("interaction: tool already registered")
	ErrToolNotFound    = errors.New("interaction: tool not found")
	ErrSessionNotFound = errors.New("interaction: session not found")
	ErrSessionClosed   = errors.New("interaction: session closed")
	ErrUnauthorized    = errors.New("interaction: unauthorized")
)
