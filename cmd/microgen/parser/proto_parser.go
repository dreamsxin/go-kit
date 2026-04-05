package parser

import (
	"os"
	"strings"

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
				Name:   r.Name,
				Input:  r.RequestType,
				Output: r.ReturnsType,
			}
			if r.Comment != nil {
				raw := strings.TrimSpace(r.Comment.Message())
				method.Doc = raw
				method.Summary = strings.SplitN(raw, "\n", 2)[0]
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
						Name:    dbschema_SnakeToCamel(f.Name),
						Type:    goType,
						JSONTag: f.Name,
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
