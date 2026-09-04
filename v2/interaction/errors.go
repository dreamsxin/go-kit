package interaction

import "errors"

// Sentinel errors returned by the interaction package.
var (
	ErrNilToolFunc           = errors.New("interaction: nil tool func")
	ErrNilTool               = errors.New("interaction: nil tool")
	ErrEmptyToolName         = errors.New("interaction: empty tool name")
	ErrToolExists            = errors.New("interaction: tool already registered")
	ErrToolNotFound          = errors.New("interaction: tool not found")
	ErrSessionNotFound       = errors.New("interaction: session not found")
	ErrSessionClosed         = errors.New("interaction: session closed")
	ErrUnauthorized          = errors.New("interaction: unauthorized")
	ErrResourceNotFound      = errors.New("interaction: resource not found")
	ErrResourceExists        = errors.New("interaction: resource already registered")
	ErrPromptNotFound        = errors.New("interaction: prompt not found")
	ErrPromptExists          = errors.New("interaction: prompt already registered")
	ErrInvalidArgument       = errors.New("interaction: invalid argument")
	ErrCompletionUnsupported = errors.New("interaction: completions not supported")
	// ErrRuntimeNotConfigured reports that a Runtime is missing a component it
	// cannot work without. Runtime's Sessions, Events, and Tools have no
	// zero-value behaviour, so a Runtime built without NewRuntime — or one whose
	// field was assigned nil — reports this instead of panicking on the call.
	ErrRuntimeNotConfigured = errors.New("interaction: runtime is missing a required component")
)
