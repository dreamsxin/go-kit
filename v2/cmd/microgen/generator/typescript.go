package generator

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
)

// TypeScriptCompilerVersion is the compiler version used by generated SDK
// documentation and the release contract check.
const TypeScriptCompilerVersion = "7.0.2"

type typeScriptSDKData struct {
	CompilerVersion string
	Messages        []typeScriptMessage
	Services        []typeScriptService
}

type typeScriptMessage struct {
	Name        string
	Description string
	Fields      []typeScriptField
}

type typeScriptField struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

type typeScriptService struct {
	Name         string
	ClientName   string
	PropertyName string
	Description  string
	Methods      []typeScriptMethod
}

type typeScriptMethod struct {
	Name          string
	Description   string
	RequestType   string
	ResponseType  string
	HTTPMethod    string
	Route         string
	HasRequest    bool
	HasBody       bool
	PathFields    []typeScriptRouteField
	QueryFields   []typeScriptRouteField
	ExampleFields []typeScriptExampleField
}

type typeScriptRouteField struct {
	Name        string
	Placeholder string
	Duration    bool
}

type typeScriptExampleField struct {
	Name  string
	Value string
}

func (g *Generator) generateTypeScriptSDK(project *ir.Project) error {
	if err := os.MkdirAll(g.layout.typeScriptSDKDir(), 0o755); err != nil {
		return err
	}
	data := buildTypeScriptSDKData(project, g.config.RoutePrefix)
	if err := g.executeTemplate("typescript_client.tmpl", g.layout.typeScriptClientFile(), data); err != nil {
		return err
	}
	if err := g.executeTemplate("typescript_readme.tmpl", g.layout.typeScriptReadme(), data); err != nil {
		return err
	}
	return g.executeTemplate("typescript_tsconfig.tmpl", g.layout.typeScriptConfig(), data)
}

func buildTypeScriptSDKData(project *ir.Project, basePrefix string) typeScriptSDKData {
	if project == nil {
		project = &ir.Project{}
	}
	messages := collectOpenAPIMessages(project)
	messageNames := contractMessageNames(messages)
	data := typeScriptSDKData{
		CompilerVersion: TypeScriptCompilerVersion,
		Messages:        make([]typeScriptMessage, 0, len(messages)),
		Services:        make([]typeScriptService, 0, len(project.Services)),
	}

	for _, message := range messages {
		if message == nil || message.Name == "" {
			continue
		}
		view := typeScriptMessage{
			Name:        message.Name,
			Description: typeScriptComment(message.Description),
		}
		for _, field := range message.Fields {
			if field == nil || field.JSONName == "-" {
				continue
			}
			view.Fields = append(view.Fields, typeScriptField{
				Name:        firstNonEmpty(field.JSONName, toSnakeCase(field.Name)),
				Type:        typeScriptType(field.GoType, field.SchemaType, messageNames),
				Description: typeScriptComment(field.Description),
				Required:    field.Required,
			})
		}
		data.Messages = append(data.Messages, view)
	}
	sort.Slice(data.Messages, func(i, j int) bool { return data.Messages[i].Name < data.Messages[j].Name })

	for _, service := range project.Services {
		if service == nil || service.Name == "" {
			continue
		}
		view := typeScriptService{
			Name:         service.Name,
			ClientName:   service.Name + "Client",
			PropertyName: lowerFirst(service.Name),
			Description:  typeScriptComment(service.Description),
		}
		prefix := routePrefix(basePrefix, service.Name)
		for _, method := range unaryMethods(service) {
			view.Methods = append(view.Methods, buildTypeScriptMethod(method, messages, messageNames, prefix))
		}
		data.Services = append(data.Services, view)
	}
	sort.Slice(data.Services, func(i, j int) bool { return data.Services[i].Name < data.Services[j].Name })
	return data
}

func buildTypeScriptMethod(method *ir.Method, messages map[string]*ir.Message, messageNames map[string]struct{}, prefix string) typeScriptMethod {
	httpMethod := strings.ToUpper(strings.TrimSpace(method.HTTPMethod))
	if httpMethod == "" {
		httpMethod = "POST"
	}
	inputName := openAPIMessageName(method.InputName, method.Input)
	outputName := openAPIMessageName(method.OutputName, method.Output)
	view := typeScriptMethod{
		Name:         lowerFirst(method.Name),
		Description:  typeScriptComment(firstNonEmpty(method.Description, method.Summary)),
		RequestType:  firstNonEmpty(inputName, "Record<string, never>"),
		ResponseType: firstNonEmpty(outputName, "void"),
		HTTPMethod:   httpMethod,
		Route:        joinOpenAPIPath(prefix, method.Route),
		HasRequest:   inputName != "",
		HasBody:      httpMethod != "GET" && inputName != "",
	}

	input := messages[inputName]
	if input == nil {
		input = method.Input
	}
	if input == nil {
		return view
	}
	for _, field := range input.Fields {
		if field == nil || field.JSONName == "-" {
			continue
		}
		name := firstNonEmpty(field.JSONName, toSnakeCase(field.Name))
		placeholder := ""
		for _, candidate := range []string{name, field.Name} {
			if strings.Contains(view.Route, "{"+candidate+"}") {
				placeholder = "{" + candidate + "}"
				break
			}
		}
		routeField := typeScriptRouteField{
			Name:        name,
			Placeholder: placeholder,
			Duration:    strings.TrimLeft(strings.TrimSpace(field.GoType), "*") == "time.Duration",
		}
		if placeholder != "" {
			view.PathFields = append(view.PathFields, routeField)
		} else if httpMethod == "GET" {
			view.QueryFields = append(view.QueryFields, routeField)
		}
		if field.Required || placeholder != "" {
			view.ExampleFields = append(view.ExampleFields, typeScriptExampleField{
				Name:  name,
				Value: typeScriptExampleValue(field, messageNames),
			})
		}
	}
	return view
}

func typeScriptType(goType, schemaType string, messageNames map[string]struct{}) string {
	t := strings.TrimSpace(goType)
	for strings.HasPrefix(t, "*") {
		t = strings.TrimPrefix(t, "*")
	}
	if t == "[]byte" {
		return "string"
	}
	if strings.HasPrefix(t, "[]") {
		return "Array<" + typeScriptType(strings.TrimPrefix(t, "[]"), "", messageNames) + ">"
	}
	if strings.HasPrefix(t, "map[") {
		valueType := "any"
		if end := strings.Index(t, "]"); end >= 0 && end+1 < len(t) {
			valueType = t[end+1:]
		}
		return "Record<string, " + typeScriptType(valueType, "", messageNames) + ">"
	}
	if _, ok := messageNames[t]; ok {
		return t
	}
	if dot := strings.LastIndex(t, "."); dot >= 0 {
		if name := t[dot+1:]; name != "" {
			if _, ok := messageNames[name]; ok {
				return name
			}
		}
	}

	switch t {
	case "string", "time.Time", "uuid.UUID":
		return "string"
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "time.Duration":
		return "number"
	case "any", "interface{}", "":
		return typeScriptSchemaFallback(schemaType)
	default:
		return typeScriptSchemaFallback(schemaType)
	}
}

func typeScriptSchemaFallback(schemaType string) string {
	switch strings.ToLower(strings.TrimSpace(schemaType)) {
	case "string":
		return "string"
	case "boolean":
		return "boolean"
	case "integer", "number":
		return "number"
	case "array":
		return "Array<unknown>"
	case "object":
		return "Record<string, unknown>"
	default:
		return "unknown"
	}
}

func lowerFirst(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	end := 1
	for end < len(runes) && unicode.IsUpper(runes[end]) {
		if end+1 < len(runes) && unicode.IsLower(runes[end+1]) {
			break
		}
		end++
	}
	for i := 0; i < end; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

func typeScriptExampleValue(field *ir.Field, messageNames map[string]struct{}) string {
	if field.Example != "" {
		var value any
		if err := json.Unmarshal([]byte(field.Example), &value); err != nil {
			value = strings.Trim(field.Example, `"`)
		}
		if data, err := json.Marshal(value); err == nil {
			return string(data)
		}
	}
	typeName := typeScriptType(field.GoType, field.SchemaType, messageNames)
	switch {
	case typeName == "string":
		return `"value"`
	case typeName == "boolean":
		return "false"
	case typeName == "number":
		return "0"
	case strings.HasPrefix(typeName, "Array<"):
		return "[]"
	case strings.HasPrefix(typeName, "Record<"):
		return "{}"
	case typeName == "unknown":
		return "undefined"
	default:
		return "{} as " + typeName
	}
}

func typeScriptComment(value string) string {
	value = strings.ReplaceAll(value, "*/", "* /")
	return strings.Join(strings.Fields(value), " ")
}
