package parser

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dreamsxin/go-kit/v2/validation"
	"github.com/emicklei/proto"
)

// ParseProto parses a .proto file into a ParseResult.
func ParseProto(protoPath string) (*ParseResult, error) {
	reader, err := os.Open(protoPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	p := proto.NewParser(reader)
	definition, err := p.Parse()
	if err != nil {
		return nil, err
	}

	result := &ParseResult{
		PackageName: "pb", // Default
		Source:      SourceProto,
	}

	var currentService *Service
	var parseErrors []error

	proto.Walk(definition,
		proto.WithPackage(func(p *proto.Package) {
			result.PackageName = p.Name
		}),
		proto.WithService(func(s *proto.Service) {
			currentService = &Service{
				ServiceName: s.Name,
				PackageName: strings.ToLower(s.Name),
				Title:       s.Name + " API",
			}
			if s.Comment != nil {
				currentService.Description = strings.TrimSpace(s.Comment.Message())
			}
			result.Services = append(result.Services, currentService)
		}),
		proto.WithRPC(func(r *proto.RPC) {
			if currentService == nil {
				return
			}
			method := Method{
				Name:          r.Name,
				Input:         r.RequestType,
				Output:        r.ReturnsType,
				StreamsInput:  r.StreamsRequest,
				StreamsOutput: r.StreamsReturns,
			}
			if r.Comment != nil {
				raw := strings.TrimSpace(r.Comment.Message())
				method.Doc = raw
				method.Summary = strings.SplitN(raw, "\n", 2)[0]
				// auth: directives parse identically on both IDL sources.
				// Comment.Message() returns only the first line of a
				// multi-line group, so the auth scan reads the raw lines.
				auth, authErr := parseAuthDoc(strings.Join(r.Comment.Lines, "\n"))
				if authErr != nil {
					parseErrors = append(parseErrors, fmt.Errorf("rpc %s: %w", r.Name, authErr))
				}
				method.Auth = auth
			}
			if method.Summary == "" {
				method.Summary = r.Name
			}
			method.Tags = currentService.ServiceName

			// Default HTTP mapping
			name := strings.ToLower(r.Name)
			switch {
			case strings.HasPrefix(name, "get") || strings.HasPrefix(name, "find") ||
				strings.HasPrefix(name, "query") || strings.HasPrefix(name, "list") ||
				strings.HasPrefix(name, "search"):
				method.HTTPMethod = "get"
			case strings.HasPrefix(name, "delete") || strings.HasPrefix(name, "remove"):
				method.HTTPMethod = "delete"
			case strings.HasPrefix(name, "update") || strings.HasPrefix(name, "edit") ||
				strings.HasPrefix(name, "modify") || strings.HasPrefix(name, "patch"):
				method.HTTPMethod = "put"
			default:
				method.HTTPMethod = "post"
			}
			method.Route = "/" + name

			currentService.Methods = append(currentService.Methods, method)
		}),
		proto.WithMessage(func(m *proto.Message) {
			model := &Model{
				Name:      m.Name,
				TableName: ToSnakeCase(m.Name),
			}
			if m.Comment != nil {
				model.Comment = strings.TrimSpace(m.Comment.Message())
			}
			for _, item := range m.Elements {
				if f, ok := item.(*proto.NormalField); ok {
					goType := protoTypeToGoType(f.Type)
					if f.Repeated {
						goType = "[]" + goType
					}
					field := ModelField{
						Name:        dbschema_SnakeToCamel(f.Name),
						Type:        goType,
						JSONTag:     f.Name,
						Constraints: constraintsFromFieldOptions(f.Options, goType),
					}
					if f.Comment != nil {
						field.Comment = strings.TrimSpace(f.Comment.Message())
					}
					model.Fields = append(model.Fields, field)
				} else if mf, ok := item.(*proto.MapField); ok {
					keyType := protoTypeToGoType(mf.KeyType)
					valType := protoTypeToGoType(mf.Type)
					field := ModelField{
						Name:    dbschema_SnakeToCamel(mf.Name),
						Type:    "map[" + keyType + "]" + valType,
						JSONTag: mf.Name,
					}
					if mf.Comment != nil {
						field.Comment = strings.TrimSpace(mf.Comment.Message())
					}
					model.Fields = append(model.Fields, field)
				}
			}
			result.Models = append(result.Models, model)
		}),
	)

	if len(parseErrors) > 0 {
		return nil, parseErrors[0]
	}

	return result, nil
}

func protoTypeToGoType(protoType string) string {
	switch protoType {
	case "double":
		return "float64"
	case "float":
		return "float32"
	case "int32":
		return "int32"
	case "int64":
		return "int64"
	case "uint32":
		return "uint32"
	case "uint64":
		return "uint64"
	case "sint32":
		return "int32"
	case "sint64":
		return "int64"
	case "fixed32":
		return "uint32"
	case "fixed64":
		return "uint64"
	case "sfixed32":
		return "int32"
	case "sfixed64":
		return "int64"
	case "bool":
		return "bool"
	case "string":
		return "string"
	case "bytes":
		return "[]byte"
	case "google.protobuf.Timestamp":
		return "time.Time"
	case "google.protobuf.Empty":
		return "struct{}"
	case "Empty":
		return "struct{}"
	default:
		// Message reference
		if strings.Contains(protoType, ".") {
			parts := strings.Split(protoType, ".")
			return "*" + parts[len(parts)-1]
		}
		return "*" + protoType
	}
}

// dbschema_SnakeToCamel is a local copy since we can't easily import from dbschema without circular dep or moving it
func dbschema_SnakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

// constraintsFromFieldOptions maps protovalidate field options onto the
// contract's constraints, so a .proto source and a Go IDL produce the same
// generated validation and the same contract keywords:
//
//	string email = 3 [(buf.validate.field).string = {min_len: 1, max_len: 64}];
//	int32 age = 4 [(buf.validate.field).int32 = {gte: 0, lte: 150}];
//	string status = 5 [(buf.validate.field).string = {in: ["active", "suspended"]}];
//
// The options may be written as one aggregate literal or as separate named
// options; both spellings parse. A rule the mapping does not know is ignored —
// protovalidate has a much larger vocabulary than the contract subset, and
// half-translating a rule would make the document promise something the
// service does not enforce.
func constraintsFromFieldOptions(options []*proto.Option, goType string) validation.Constraints {
	var cons validation.Constraints
	for _, option := range options {
		if !strings.HasPrefix(option.Name, "(buf.validate.field)") {
			continue
		}
		// Two spellings exist: the whole rule in the option name
		// (...string.min_len = 1), or the rules aggregated in one map literal
		// (...string = {min_len: 1, max_len: 64}). Both arrive here.
		suffix := strings.TrimPrefix(option.Name, "(buf.validate.field)")
		suffix = strings.TrimPrefix(suffix, ".")
		if suffix == "" {
			continue
		}
		if isValidateRule(suffix) {
			applyValidateLiteral(&cons, suffix, option.Constant, goType)
			continue
		}
		// A separate named option (...string.min_len = 5) ends with the rule
		// name; its constant is the scalar value.
		segments := strings.Split(suffix, ".")
		if isValidateRule(segments[len(segments)-1]) {
			applyValidateLiteral(&cons, segments[len(segments)-1], option.Constant, goType)
			continue
		}
		for _, named := range option.Constant.OrderedMap {
			if isValidateRule(named.Name) {
				applyValidateLiteral(&cons, named.Name, *named.Literal, goType)
			}
		}
	}
	return cons
}

// isValidateRule reports whether the option suffix names a rule the mapping
// knows. A suffix that names something else (a type segment of an aggregate)
// is walked one level deeper instead.
func isValidateRule(name string) bool {
	switch name {
	case "min_len", "max_len", "gte", "lte", "in":
		return true
	}
	return false
}

// applyValidateLiteral applies one protovalidate rule to the constraints.
// Numeric rules parse against the field's Go type; a malformed value is
// ignored rather than fatal, because the option came from a compiled proto.
func applyValidateLiteral(cons *validation.Constraints, rule string, literal proto.Literal, goType string) {
	value := strings.Trim(literal.Source, `"`)
	switch {
	case rule == "min_len" && isTextualGoType(goType):
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			cons.MinLen = &n
		}
	case rule == "max_len" && isTextualGoType(goType):
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			cons.MaxLen = &n
		}
	case (rule == "gte" || rule == "greater_than") && isNumericGoType(goType):
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			cons.Min = &f
		}
	case (rule == "lte" || rule == "less_than") && isNumericGoType(goType):
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			cons.Max = &f
		}
	case rule == "in":
		if literal.Array != nil {
			enum := make([]string, 0, len(literal.Array))
			for _, entry := range literal.Array {
				enum = append(enum, strings.Trim(entry.Source, `"`))
			}
			cons.Enum = enum
		}
	}
}

func isNumericGoType(goType string) bool {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(goType), "*"))
	switch t {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return true
	}
	return false
}

func isTextualGoType(goType string) bool {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(goType), "*")) == "string"
}
