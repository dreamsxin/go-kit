package ir

import (
	"strings"

	"github.com/dreamsxin/go-kit/cmd/microgen/parser"
)

// FromParseResult converts the current parser output into the source-agnostic
// microgen intermediate representation.
func FromParseResult(result *parser.ParseResult) *Project {
	if result == nil {
		return &Project{}
	}

	project := &Project{
		PackageName: result.PackageName,
		Source:      string(result.Source),
	}

	messageByName := make(map[string]*Message, len(result.Models))
	for _, model := range result.Models {
		msg := &Message{
			Name:        model.Name,
			TableName:   model.TableName,
			Description: model.Comment,
			HasGormTags: model.HasGormTags,
		}
		for _, field := range model.Fields {
			msg.Fields = append(msg.Fields, &Field{
				Name:        field.Name,
				JSONName:    fieldJSONName(field),
				GoType:      field.Type,
				SchemaType:  goTypeToSchemaType(field.Type),
				GormTag:     field.GormTag,
				Description: field.Comment,
				Required:    field.IsNotNull || !strings.HasPrefix(field.Type, "*"),
				IsPrimary:   field.IsPrimary,
				IsAutoIncr:  field.IsAutoIncr,
				IsUnique:    field.IsUnique,
				SwagType:    field.SwagType,
				Example:     field.Example,
			})
		}
		project.Messages = append(project.Messages, msg)
		messageByName[msg.Name] = msg
	}

	for _, service := range result.Services {
		svc := &Service{
			Name:        service.ServiceName,
			PackageName: service.PackageName,
			Title:       service.Title,
			Description: service.Description,
		}
		for _, method := range service.Methods {
			m := &Method{
				Name:        method.Name,
				Summary:     method.Summary,
				Description: method.Doc,
				HTTPMethod:  strings.ToUpper(method.HTTPMethod),
				Route:       method.Route,
				Tags:        compactTags(method.Tags),
				InputName:   trimTypeRef(method.Input),
				OutputName:  trimTypeRef(method.Output),
			}
			m.Input = messageByName[m.InputName]
			m.Output = messageByName[m.OutputName]
			svc.Methods = append(svc.Methods, m)
		}
		project.Services = append(project.Services, svc)
	}

	return project
}

func fieldJSONName(field parser.ModelField) string {
	name := strings.Split(field.JSONTag, ",")[0]
	if name == "" {
		return parser.ToSnakeCase(field.Name)
	}
	return name
}

func trimTypeRef(goType string) string {
	return strings.TrimPrefix(goType, "*")
}

func compactTags(tags string) []string {
	if tags == "" {
		return nil
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func goTypeToSchemaType(goType string) string {
	t := strings.TrimPrefix(goType, "*")
	switch {
	case t == "bool":
		return "boolean"
	case t == "string":
		return "string"
	case strings.HasPrefix(t, "int"), strings.HasPrefix(t, "uint"):
		return "integer"
	case strings.HasPrefix(t, "float"):
		return "number"
	case strings.HasPrefix(t, "[]"):
		return "array"
	case strings.HasPrefix(t, "map["):
		return "object"
	default:
		return "object"
	}
}
