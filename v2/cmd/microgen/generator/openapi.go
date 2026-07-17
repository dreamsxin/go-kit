package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"
)

const (
	openAPIVersion   = "3.1.0"
	openAPIRefPrefix = "#/components/schemas/"
)

type openAPIDocument struct {
	OpenAPI    string                 `json:"openapi"`
	Info       openAPIInfo            `json:"info"`
	Tags       []openAPITag           `json:"tags,omitempty"`
	Paths      map[string]openAPIPath `json:"paths"`
	Components openAPIComponents      `json:"components"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type openAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type openAPIPath map[string]openAPIOperation

type openAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	Tags        []string                   `json:"tags,omitempty"`
	Parameters  []openAPIParameter         `json:"parameters,omitempty"`
	RequestBody *openAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Style       string         `json:"style,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`
	Schema      *openAPISchema `json:"schema"`
}

type openAPIRequestBody struct {
	Required bool                    `json:"required"`
	Content  map[string]openAPIMedia `json:"content"`
}

type openAPIResponse struct {
	Description string                  `json:"description"`
	Content     map[string]openAPIMedia `json:"content,omitempty"`
}

type openAPIMedia struct {
	Schema *openAPISchema `json:"schema"`
}

type openAPIComponents struct {
	Schemas map[string]*openAPISchema `json:"schemas"`
}

type openAPISchema struct {
	Ref                  string                    `json:"$ref,omitempty"`
	Type                 string                    `json:"type,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Properties           map[string]*openAPISchema `json:"properties,omitempty"`
	Items                *openAPISchema            `json:"items,omitempty"`
	AdditionalProperties any                       `json:"additionalProperties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	Example              any                       `json:"example,omitempty"`
}

func (g *Generator) generateContracts(ctx generationContext) error {
	if err := os.MkdirAll(g.layout.docsDir(), 0o755); err != nil {
		return err
	}
	if err := g.executeTemplate("docs.tmpl", g.layout.docsEmbed(), struct{}{}); err != nil {
		return err
	}
	doc := buildOpenAPIDocument(ctx.project, g.config.RoutePrefix)
	if err := writeJSONDocument(g.layout.openAPIFile(), doc); err != nil {
		return fmt.Errorf("write OpenAPI document: %w", err)
	}
	if err := writeJSONDocument(g.layout.jsonSchemaFile(), buildJSONSchemaDocument(ctx.project)); err != nil {
		return fmt.Errorf("write JSON Schema document: %w", err)
	}
	return nil
}

func writeJSONDocument(path string, document any) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func buildOpenAPIDocument(project *ir.Project, basePrefix string) openAPIDocument {
	if project == nil {
		project = &ir.Project{}
	}
	messages := collectOpenAPIMessages(project)
	messageNames := contractMessageNames(messages)
	schemas := buildContractSchemas(messages, messageNames, openAPIRefPrefix)

	doc := openAPIDocument{
		OpenAPI: openAPIVersion,
		Info: openAPIInfo{
			Title:       openAPITitle(project),
			Description: openAPIDescription(project),
			Version:     "1.0.0",
		},
		Paths:      map[string]openAPIPath{},
		Components: openAPIComponents{Schemas: schemas},
	}

	for _, service := range project.Services {
		if service == nil {
			continue
		}
		doc.Tags = append(doc.Tags, openAPITag{Name: service.Name, Description: service.Description})
		prefix := routePrefix(basePrefix, service.Name)
		for _, method := range unaryMethods(service) {
			path := joinOpenAPIPath(prefix, method.Route)
			verb := strings.ToLower(method.HTTPMethod)
			if verb == "" {
				verb = "post"
			}
			item := doc.Paths[path]
			if item == nil {
				item = openAPIPath{}
			}
			item[verb] = openAPIOperationFor(service, method, path, messageNames)
			doc.Paths[path] = item
		}
	}
	sort.Slice(doc.Tags, func(i, j int) bool { return doc.Tags[i].Name < doc.Tags[j].Name })
	return doc
}

func openAPIOperationFor(service *ir.Service, method *ir.Method, path string, messageNames map[string]struct{}) openAPIOperation {
	inputName := openAPIMessageName(method.InputName, method.Input)
	outputName := openAPIMessageName(method.OutputName, method.Output)
	tags := append([]string(nil), method.Tags...)
	if len(tags) == 0 {
		tags = []string{service.Name}
	}
	operation := openAPIOperation{
		OperationID: service.Name + "_" + method.Name,
		Summary:     firstNonEmpty(method.Summary, method.Name),
		Description: method.Description,
		Tags:        tags,
		Responses: map[string]openAPIResponse{
			"200": successOpenAPIResponse(outputName),
			"400": errorOpenAPIResponse("Invalid request"),
			"500": errorOpenAPIResponse("Internal server error"),
		},
	}
	if strings.EqualFold(method.HTTPMethod, "GET") {
		operation.Parameters = openAPIQueryParameters(method.Input, path, messageNames)
	} else if inputName != "" {
		operation.Parameters = openAPIPathParameters(method.Input, path, messageNames)
		operation.RequestBody = &openAPIRequestBody{
			Required: true,
			Content: map[string]openAPIMedia{
				"application/json": {Schema: openAPIRef(inputName)},
			},
		}
	}
	return operation
}

func openAPIMessageName(explicit string, message *ir.Message) string {
	if explicit != "" {
		return explicit
	}
	if message != nil {
		return message.Name
	}
	return ""
}

func openAPIPathParameters(message *ir.Message, path string, messageNames map[string]struct{}) []openAPIParameter {
	parameters := openAPIQueryParameters(message, path, messageNames)
	pathParameters := parameters[:0]
	for _, parameter := range parameters {
		if parameter.In == "path" {
			pathParameters = append(pathParameters, parameter)
		}
	}
	return pathParameters
}

func openAPIQueryParameters(message *ir.Message, path string, messageNames map[string]struct{}) []openAPIParameter {
	if message == nil {
		return nil
	}
	parameters := make([]openAPIParameter, 0, len(message.Fields))
	for _, field := range message.Fields {
		if field == nil || field.JSONName == "-" {
			continue
		}
		name := firstNonEmpty(field.JSONName, strings.ToLower(field.Name))
		location := "query"
		required := false
		if strings.Contains(path, "{"+name+"}") || strings.Contains(path, "{"+field.Name+"}") {
			location = "path"
			required = true
		}
		schema := openAPIFieldSchema(field, messageNames)
		if location == "query" && strings.TrimLeft(strings.TrimSpace(field.GoType), "*") == "time.Duration" {
			schema = &openAPISchema{Type: "string", Description: field.Description, Example: "1s"}
		}
		parameter := openAPIParameter{
			Name:        name,
			In:          location,
			Description: field.Description,
			Required:    required,
			Schema:      schema,
		}
		if schema.Type == "array" && location == "query" {
			explode := true
			parameter.Style = "form"
			parameter.Explode = &explode
		}
		parameters = append(parameters, parameter)
	}
	return parameters
}

func successOpenAPIResponse(outputName string) openAPIResponse {
	response := openAPIResponse{Description: "Success"}
	if outputName != "" {
		response.Content = map[string]openAPIMedia{
			"application/json": {Schema: openAPIRef(outputName)},
		}
	}
	return response
}

func errorOpenAPIResponse(description string) openAPIResponse {
	return openAPIResponse{
		Description: description,
		Content: map[string]openAPIMedia{
			"application/json": {Schema: openAPIRef("ErrorResponse")},
		},
	}
}

func openAPIMessageSchema(message *ir.Message, messageNames map[string]struct{}) *openAPISchema {
	return contractMessageSchema(message, messageNames, openAPIRefPrefix)
}

func contractMessageSchema(message *ir.Message, messageNames map[string]struct{}, refPrefix string) *openAPISchema {
	schema := &openAPISchema{Type: "object", Properties: map[string]*openAPISchema{}}
	if message == nil {
		return schema
	}
	schema.Description = message.Description
	for _, field := range message.Fields {
		if field == nil || field.JSONName == "-" {
			continue
		}
		name := firstNonEmpty(field.JSONName, strings.ToLower(field.Name))
		schema.Properties[name] = contractFieldSchema(field, messageNames, refPrefix)
		if field.Required {
			schema.Required = append(schema.Required, name)
		}
	}
	sort.Strings(schema.Required)
	return schema
}

func openAPIFieldSchema(field *ir.Field, messageNames map[string]struct{}) *openAPISchema {
	return contractFieldSchema(field, messageNames, openAPIRefPrefix)
}

func contractFieldSchema(field *ir.Field, messageNames map[string]struct{}, refPrefix string) *openAPISchema {
	schema := contractSchemaForType(field.GoType, field.SchemaType, messageNames, refPrefix)
	schema.Description = field.Description
	if field.Example != "" {
		var example any
		if err := json.Unmarshal([]byte(field.Example), &example); err != nil {
			example = strings.Trim(field.Example, `"`)
		}
		schema.Example = example
	}
	return schema
}

func openAPISchemaForType(goType, schemaType string, messageNames map[string]struct{}) *openAPISchema {
	return contractSchemaForType(goType, schemaType, messageNames, openAPIRefPrefix)
}

func contractSchemaForType(goType, schemaType string, messageNames map[string]struct{}, refPrefix string) *openAPISchema {
	t := strings.TrimSpace(goType)
	for strings.HasPrefix(t, "*") {
		t = strings.TrimPrefix(t, "*")
	}
	if t == "[]byte" {
		return &openAPISchema{Type: "string", Format: "byte"}
	}
	if strings.HasPrefix(t, "[]") {
		return &openAPISchema{Type: "array", Items: contractSchemaForType(strings.TrimPrefix(t, "[]"), "", messageNames, refPrefix)}
	}
	if strings.HasPrefix(t, "map[") {
		valueType := "any"
		if end := strings.Index(t, "]"); end >= 0 && end+1 < len(t) {
			valueType = t[end+1:]
		}
		return &openAPISchema{Type: "object", AdditionalProperties: contractSchemaForType(valueType, "", messageNames, refPrefix)}
	}
	if _, ok := messageNames[t]; ok {
		return contractSchemaRef(refPrefix, t)
	}

	switch t {
	case "string":
		return &openAPISchema{Type: "string"}
	case "bool":
		return &openAPISchema{Type: "boolean"}
	case "int8", "int16", "int32", "uint8", "uint16", "uint32":
		return &openAPISchema{Type: "integer", Format: "int32"}
	case "int", "int64", "uint", "uint64", "time.Duration":
		return &openAPISchema{Type: "integer", Format: "int64"}
	case "float32":
		return &openAPISchema{Type: "number", Format: "float"}
	case "float64":
		return &openAPISchema{Type: "number", Format: "double"}
	case "time.Time":
		return &openAPISchema{Type: "string", Format: "date-time"}
	case "uuid.UUID":
		return &openAPISchema{Type: "string", Format: "uuid"}
	case "any", "interface{}", "":
		return &openAPISchema{Type: firstNonEmpty(schemaType, "object")}
	}
	return &openAPISchema{Type: firstNonEmpty(schemaType, "object")}
}

func openAPIRef(name string) *openAPISchema {
	return contractSchemaRef(openAPIRefPrefix, name)
}

func contractSchemaRef(refPrefix, name string) *openAPISchema {
	return &openAPISchema{Ref: refPrefix + name}
}

func contractMessageNames(messages map[string]*ir.Message) map[string]struct{} {
	names := make(map[string]struct{}, len(messages))
	for name := range messages {
		names[name] = struct{}{}
	}
	return names
}

func buildContractSchemas(messages map[string]*ir.Message, messageNames map[string]struct{}, refPrefix string) map[string]*openAPISchema {
	schemas := make(map[string]*openAPISchema, len(messages)+1)
	for name, message := range messages {
		schemas[name] = contractMessageSchema(message, messageNames, refPrefix)
	}
	schemas["ErrorResponse"] = errorResponseSchema()
	return schemas
}

func errorResponseSchema() *openAPISchema {
	return &openAPISchema{
		Type: "object",
		Properties: map[string]*openAPISchema{
			"code":       {Type: "string"},
			"message":    {Type: "string"},
			"request_id": {Type: "string"},
		},
		Required: []string{"code", "message"},
	}
}

func collectOpenAPIMessages(project *ir.Project) map[string]*ir.Message {
	messages := map[string]*ir.Message{}
	for _, message := range project.Messages {
		if message != nil && message.Name != "" {
			messages[message.Name] = message
		}
	}
	for _, service := range project.Services {
		if service == nil {
			continue
		}
		for _, method := range service.Methods {
			if method == nil {
				continue
			}
			if method.Input != nil && method.Input.Name != "" {
				messages[method.Input.Name] = method.Input
			}
			if method.Output != nil && method.Output.Name != "" {
				messages[method.Output.Name] = method.Output
			}
			if method.InputName != "" {
				if _, ok := messages[method.InputName]; !ok {
					messages[method.InputName] = &ir.Message{Name: method.InputName}
				}
			}
			if method.OutputName != "" {
				if _, ok := messages[method.OutputName]; !ok {
					messages[method.OutputName] = &ir.Message{Name: method.OutputName}
				}
			}
		}
	}
	return messages
}

func openAPITitle(project *ir.Project) string {
	if len(project.Services) == 1 && project.Services[0] != nil {
		return firstNonEmpty(project.Services[0].Title, project.Services[0].Name+" API")
	}
	return firstNonEmpty(project.PackageName, "Microservice") + " API"
}

func openAPIDescription(project *ir.Project) string {
	if len(project.Services) == 1 && project.Services[0] != nil {
		return firstNonEmpty(project.Services[0].Description, project.Services[0].Name+" service API")
	}
	return "Generated service API"
}

func joinOpenAPIPath(prefix, route string) string {
	path := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(route, "/")
	if path == "" {
		return "/"
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
