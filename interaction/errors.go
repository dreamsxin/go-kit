package interaction

import "errors"

// Sentinel errors returned by the interaction package.
var (
	ErrNilToolFunc       = errors.New("interaction: nil tool func")
	ErrNilTool           = errors.New("interaction: nil tool")
	ErrEmptyToolName     = errors.New("interaction: empty tool name")
	ErrToolExists        = errors.New("interaction: tool already registered")
	ErrToolNotFound      = errors.New("interaction: tool not found")
	ErrSessionNotFound   = errors.New("interaction: session not found")
	ErrSessionClosed     = errors.New("interaction: session closed")
	ErrUnauthorized      = errors.New("interaction: unauthorized")
	ErrResourceNotFound  = errors.New("interaction: resource not found")
	ErrResourceExists    = errors.New("interaction: resource already registered")
	ErrPromptNotFound    = errors.New("interaction: prompt not found")
	ErrPromptExists      = errors.New("interaction: prompt already registered")
	ErrInvalidArgument   = errors.New("interaction: invalid argument")
	ErrCompletionUnsupported = errors.New("interaction: completions not supported")
)
