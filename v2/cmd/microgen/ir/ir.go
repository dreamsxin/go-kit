package ir

import "strings"

// MethodKind describes the interaction shape of a service method.
type MethodKind string

const (
	MethodKindUnary         MethodKind = "unary"
	MethodKindServerStream  MethodKind = "server_stream"
	MethodKindClientStream  MethodKind = "client_stream"
	MethodKindBidirectional MethodKind = "bidi_stream"
)

// Project is the source-agnostic intermediate representation used by microgen.
// It intentionally models service contracts, messages, and fields without
// committing to a specific input source such as Go IDL, proto, or DB schema.
type Project struct {
	PackageName string
	Source      string
	Services    []*Service
	Messages    []*Message
}

// Service describes a generated service contract.
type Service struct {
	Name        string
	PackageName string
	Title       string
	Description string
	Methods     []*Method
}

// Method describes a single service method.
type Method struct {
	Name        string
	Summary     string
	Description string
	Kind        MethodKind
	HTTPMethod  string
	Route       string
	Tags        []string
	InputName   string
	OutputName  string
	Input       *Message
	Output      *Message
}

// IsStreaming reports whether the method uses a long-lived stream shape.
func (m *Method) IsStreaming() bool {
	if m == nil {
		return false
	}
	switch m.Kind {
	case MethodKindServerStream, MethodKindClientStream, MethodKindBidirectional:
		return true
	default:
		return false
	}
}

// StreamsInput reports whether the request side of the method is streamed.
func (m *Method) StreamsInput() bool {
	if m == nil {
		return false
	}
	return m.Kind == MethodKindClientStream || m.Kind == MethodKindBidirectional
}

// StreamsOutput reports whether the response side of the method is streamed.
func (m *Method) StreamsOutput() bool {
	if m == nil {
		return false
	}
	return m.Kind == MethodKindServerStream || m.Kind == MethodKindBidirectional
}

// Message describes a request/response payload or model shape.
type Message struct {
	Name        string
	TableName   string
	Description string
	HasGormTags bool
	Fields      []*Field
}

// Field describes a structured payload field in source-neutral form.
type Field struct {
	Name        string
	JSONName    string
	GoType      string
	SchemaType  string
	GormTag     string
	Description string
	Required    bool
	IsPrimary   bool
	IsAutoIncr  bool
	IsUnique    bool
	Example     string
}

// HasFields reports whether the message contains any fields.
func (m *Message) HasFields() bool {
	return m != nil && len(m.Fields) > 0
}

// TagString joins the method tags in the form used by templates and docs.
func (m *Method) TagString() string {
	return strings.Join(m.Tags, ", ")
}
