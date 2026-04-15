package generator

import (
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/ir"
	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

type protoMessage struct {
	Name        string
	Comment     string
	Fields      []protoField
	Placeholder bool
}

type protoSchema struct {
	Messages       []protoMessage
	NeedsTimestamp bool
	NeedsDuration  bool
}

type protoField struct {
	Name      string
	Type      string
	KeyType   string
	ValueType string
	Comment   string
	Number    int
	Repeated  bool
	IsMap     bool
	Optional  bool
}

func buildProtoSchema(service *serviceView, models []*modelView) protoSchema {
	index := make(map[string]*modelView, len(models))
	for _, model := range models {
		index[strings.TrimPrefix(model.Name, "*")] = model
	}

	var schema protoSchema
	seen := map[string]bool{}
	for _, method := range service.Methods {
		collectProtoMessage(strings.TrimPrefix(method.Input, "*"), index, seen, &schema)
		collectProtoMessage(strings.TrimPrefix(method.Output, "*"), index, seen, &schema)
	}
	return schema
}

func buildProtoSchemaFromIR(service *ir.Service, messages []*ir.Message) protoSchema {
	index := make(map[string]*ir.Message, len(messages))
	for _, message := range messages {
		index[strings.TrimPrefix(message.Name, "*")] = message
	}

	var schema protoSchema
	seen := map[string]bool{}
	for _, method := range service.Methods {
		collectProtoIRMessage(strings.TrimPrefix(method.InputName, "*"), index, seen, &schema)
		collectProtoIRMessage(strings.TrimPrefix(method.OutputName, "*"), index, seen, &schema)
	}
	return schema
}

func collectProtoMessage(name string, index map[string]*modelView, seen map[string]bool, schema *protoSchema) {
	name = strings.TrimPrefix(name, "*")
	if name == "" || seen[name] || isProtoScalarTypeName(name) {
		return
	}
	seen[name] = true

	model, ok := index[name]
	if !ok {
		schema.Messages = append(schema.Messages, protoMessage{
			Name:        name,
			Placeholder: true,
		})
		return
	}

	msg := protoMessage{
		Name:    name,
		Comment: model.Comment,
	}

	for i, field := range model.Fields {
		pf, refs, ok := protoFieldFromModelField(field, i+1)
		if !ok {
			continue
		}
		msg.Fields = append(msg.Fields, pf)
		if pf.Type == "google.protobuf.Timestamp" || pf.ValueType == "google.protobuf.Timestamp" {
			schema.NeedsTimestamp = true
		}
		if pf.Type == "google.protobuf.Duration" || pf.ValueType == "google.protobuf.Duration" {
			schema.NeedsDuration = true
		}
		for _, ref := range refs {
			collectProtoMessage(ref, index, seen, schema)
		}
	}

	if len(msg.Fields) == 0 {
		msg.Placeholder = true
	}

	schema.Messages = append(schema.Messages, msg)
}

func collectProtoIRMessage(name string, index map[string]*ir.Message, seen map[string]bool, schema *protoSchema) {
	name = strings.TrimPrefix(name, "*")
	if name == "" || seen[name] || isProtoScalarTypeName(name) {
		return
	}
	seen[name] = true

	message, ok := index[name]
	if !ok {
		schema.Messages = append(schema.Messages, protoMessage{
			Name:        name,
			Placeholder: true,
		})
		return
	}

	msg := protoMessage{
		Name:    message.Name,
		Comment: message.Description,
	}

	for i, field := range message.Fields {
		pf, refs, ok := protoFieldFromIRField(field, i+1)
		if !ok {
			continue
		}
		msg.Fields = append(msg.Fields, pf)
		if pf.Type == "google.protobuf.Timestamp" || pf.ValueType == "google.protobuf.Timestamp" {
			schema.NeedsTimestamp = true
		}
		if pf.Type == "google.protobuf.Duration" || pf.ValueType == "google.protobuf.Duration" {
			schema.NeedsDuration = true
		}
		for _, ref := range refs {
			collectProtoIRMessage(ref, index, seen, schema)
		}
	}

	if len(msg.Fields) == 0 {
		msg.Placeholder = true
	}

	schema.Messages = append(schema.Messages, msg)
}

func protoFieldFromModelField(field modelFieldView, number int) (protoField, []string, bool) {
	name := field.JSONTag
	if idx := strings.Index(name, ","); idx >= 0 {
		name = name[:idx]
	}
	if name == "" || name == "-" {
		name = parser.ToSnakeCase(field.Name)
	}

	pf := protoField{
		Name:    name,
		Comment: field.Comment,
		Number:  number,
	}

	isPointer := strings.HasPrefix(strings.TrimSpace(field.Type), "*")
	goType := strings.TrimPrefix(field.Type, "*")
	switch {
	case goType == "[]byte":
		pf.Type = "bytes"
		return pf, nil, true
	case strings.HasPrefix(goType, "[]"):
		elemType, refs := protoTypeFromGoType(goType[2:])
		if elemType == "" {
			return protoField{}, nil, false
		}
		pf.Repeated = true
		pf.Type = elemType
		return pf, refs, true
	case strings.HasPrefix(goType, "map["):
		keyType, valueType, refs, ok := protoMapTypes(goType)
		if !ok {
			return protoField{}, nil, false
		}
		pf.IsMap = true
		pf.KeyType = keyType
		pf.ValueType = valueType
		return pf, refs, true
	default:
		fieldType, refs := protoTypeFromGoType(goType)
		if fieldType == "" {
			return protoField{}, nil, false
		}
		pf.Type = fieldType
		pf.Optional = isPointer && isProtoOptionalType(fieldType)
		return pf, refs, true
	}
}

func protoFieldFromIRField(field *ir.Field, number int) (protoField, []string, bool) {
	if field == nil {
		return protoField{}, nil, false
	}

	name := field.JSONName
	if name == "" || name == "-" {
		name = parser.ToSnakeCase(field.Name)
	}

	pf := protoField{
		Name:    name,
		Comment: field.Description,
		Number:  number,
	}

	isPointer := strings.HasPrefix(strings.TrimSpace(field.GoType), "*")
	goType := strings.TrimPrefix(field.GoType, "*")
	switch {
	case goType == "[]byte":
		pf.Type = "bytes"
		return pf, nil, true
	case strings.HasPrefix(goType, "[]"):
		elemType, refs := protoTypeFromGoType(goType[2:])
		if elemType == "" {
			return protoField{}, nil, false
		}
		pf.Repeated = true
		pf.Type = elemType
		return pf, refs, true
	case strings.HasPrefix(goType, "map["):
		keyType, valueType, refs, ok := protoMapTypes(goType)
		if !ok {
			return protoField{}, nil, false
		}
		pf.IsMap = true
		pf.KeyType = keyType
		pf.ValueType = valueType
		return pf, refs, true
	default:
		fieldType, refs := protoTypeFromGoType(goType)
		if fieldType == "" {
			return protoField{}, nil, false
		}
		pf.Type = fieldType
		pf.Optional = isPointer && isProtoOptionalType(fieldType)
		return pf, refs, true
	}
}

func protoMapTypes(goType string) (string, string, []string, bool) {
	body := strings.TrimPrefix(goType, "map[")
	end := strings.Index(body, "]")
	if end < 0 {
		return "", "", nil, false
	}

	keyType, _ := protoTypeFromGoType(body[:end])
	valueType, refs := protoTypeFromGoType(body[end+1:])
	if keyType == "" || valueType == "" || !isProtoMapKeyType(keyType) {
		return "", "", nil, false
	}
	return keyType, valueType, refs, true
}

func protoTypeFromGoType(goType string) (string, []string) {
	t := strings.TrimPrefix(strings.TrimSpace(goType), "*")
	switch t {
	case "string":
		return "string", nil
	case "bool":
		return "bool", nil
	case "int8", "int16", "int32":
		return "int32", nil
	case "int", "int64":
		return "int64", nil
	case "uint8", "uint16", "uint32":
		return "uint32", nil
	case "uint", "uint64":
		return "uint64", nil
	case "float32":
		return "float", nil
	case "float64":
		return "double", nil
	case "[]byte":
		return "bytes", nil
	case "time.Time":
		return "google.protobuf.Timestamp", nil
	case "time.Duration":
		return "google.protobuf.Duration", nil
	case "interface{}", "any":
		return "string", nil
	case "struct{}":
		return "", nil
	default:
		if strings.Contains(t, ".") {
			parts := strings.Split(t, ".")
			t = parts[len(parts)-1]
		}
		if t == "" {
			return "", nil
		}
		return t, []string{t}
	}
}

func isProtoScalarTypeName(name string) bool {
	switch name {
	case "string", "bool", "bytes", "double", "float",
		"int32", "int64", "uint32", "uint64",
		"sint32", "sint64", "fixed32", "fixed64",
		"sfixed32", "sfixed64":
		return true
	default:
		return false
	}
}

func isProtoMapKeyType(name string) bool {
	switch name {
	case "int32", "int64", "uint32", "uint64", "bool", "string":
		return true
	default:
		return false
	}
}

func isProtoOptionalType(name string) bool {
	switch name {
	case "string", "bool", "bytes", "double", "float",
		"int32", "int64", "uint32", "uint64",
		"sint32", "sint64", "fixed32", "fixed64",
		"sfixed32", "sfixed64":
		return true
	default:
		return false
	}
}
