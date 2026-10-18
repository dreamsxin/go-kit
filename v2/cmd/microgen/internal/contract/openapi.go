package contract

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/ir"
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
	Security    []map[string][]string      `json:"security,omitempty"`
}

// Stable: openapi.security-scheme — a method whose IDL declares auth carries
// security: [{auth: []}], the components declare the http bearer scheme
// named auth, and the operation's responses document 401 (and 403 when roles
// are listed) with the shared error schema.
// Covered by: TestOpenAPISecurityDeclaredForSecuredMethods,
// TestOpenAPISecurityAbsentForUnsecuredMethods

type openAPISecurityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
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
	Schemas         map[string]*openAPISchema        `json:"schemas"`
	SecuritySchemes map[string]openAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type openAPISchema struct {
	Ref string `json:"$ref,omitempty"`
	// Type is a string, or a []string when the value may also be null. OpenAPI
	// 3.1 is JSON Schema 2020-12, which has no `nullable` keyword: null is a type
	// like any other, so admitting it means listing two.
	Type                 any                       `json:"type,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Properties           map[string]*openAPISchema `json:"properties,omitempty"`
	Items                *openAPISchema            `json:"items,omitempty"`
	AnyOf                []*openAPISchema          `json:"anyOf,omitempty"`
	AdditionalProperties any                       `json:"additionalProperties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	Example              any                       `json:"example,omitempty"`
	Enum                 []any                     `json:"enum,omitempty"`
	Minimum              *float64                  `json:"minimum,omitempty"`
	Maximum              *float64                  `json:"maximum,omitempty"`
	MinLength            *int                      `json:"minLength,omitempty"`
	MaxLength            *int                      `json:"maxLength,omitempty"`
	Default              any                       `json:"default,omitempty"`
}

func BuildOpenAPIDocument(project *ir.Project, basePrefix string) openAPIDocument {
	if project == nil {
		project = &ir.Project{}
	}
	messages := CollectOpenAPIMessages(project)
	messageNames := ContractMessageNames(messages)
	schemas := buildContractSchemas(messages, messageNames, openAPIRefPrefix)

	doc := openAPIDocument{
		OpenAPI: openAPIVersion,
		Info: openAPIInfo{
			Title:       openAPITitle(project),
			Description: openAPIDescription(project),
			Version:     "1.0.0",
		},
		Paths:      map[string]openAPIPath{},
		Components: openAPIComponents{Schemas: schemas, SecuritySchemes: securitySchemesFor(project)},
	}

	for _, service := range project.Services {
		if service == nil {
			continue
		}
		doc.Tags = append(doc.Tags, openAPITag{Name: service.Name, Description: service.Description})
		prefix := ir.RoutePrefix(basePrefix, service.Name)
		for _, method := range ir.UnaryMethods(service) {
			path := JoinOpenAPIPath(prefix, method.Route)
			verb := strings.ToLower(method.HTTPMethod)
			if verb == "" {
				verb = "post"
			}
			item := doc.Paths[path]
			if item == nil {
				item = openAPIPath{}
			}
			item[verb] = openAPIOperationFor(service, method, path, messages, messageNames)
			doc.Paths[path] = item
		}
	}
	sort.Slice(doc.Tags, func(i, j int) bool { return doc.Tags[i].Name < doc.Tags[j].Name })
	return doc
}

func openAPIOperationFor(service *ir.Service, method *ir.Method, path string, messages map[string]*ir.Message, messageNames map[string]struct{}) openAPIOperation {
	inputName := OpenAPIMessageName(method.InputName, method.Input)
	outputName := OpenAPIMessageName(method.OutputName, method.Output)
	input := messages[inputName]
	if input == nil {
		input = method.Input
	}
	tags := append([]string(nil), method.Tags...)
	if len(tags) == 0 {
		tags = []string{service.Name}
	}
	operation := openAPIOperation{
		OperationID: service.Name + "_" + method.Name,
		Summary:     FirstNonEmpty(method.Summary, method.Name),
		Description: method.Description,
		Tags:        tags,
		Responses: map[string]openAPIResponse{
			"200": successOpenAPIResponse(outputName),
			"400": errorOpenAPIResponse("Invalid request"),
			"500": errorOpenAPIResponse("Internal server error"),
		},
	}
	if method.Auth.Declared() {
		operation.Security = []map[string][]string{{"auth": {}}}
		operation.Responses["401"] = errorOpenAPIResponse("Unauthenticated")
		if len(method.Auth.Roles) > 0 {
			operation.Responses["403"] = errorOpenAPIResponse("Forbidden")
		}
	}
	if strings.EqualFold(method.HTTPMethod, "GET") {
		operation.Parameters = openAPIQueryParameters(input, path, messageNames)
	} else if inputName != "" {
		operation.Parameters = openAPIPathParameters(input, path, messageNames)
		operation.RequestBody = &openAPIRequestBody{
			Required: true,
			Content: map[string]openAPIMedia{
				"application/json": {Schema: openAPIRef(inputName)},
			},
		}
	}
	return operation
}

func OpenAPIMessageName(explicit string, message *ir.Message) string {
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
		name := FirstNonEmpty(field.JSONName, strings.ToLower(field.Name))
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

// securitySchemesFor declares the scheme the generated enforcement expects:
// an authenticated subject established by the application's authenticator.
// The wire credential (bearer token, API key) is the deployment's choice; the
// document carries the http bearer shape as the default and the deployment
// adjusts it when its authenticator speaks something else.
func securitySchemesFor(project *ir.Project) map[string]openAPISecurityScheme {
	for _, service := range project.Services {
		for _, method := range service.Methods {
			if method.Auth.Declared() {
				return map[string]openAPISecurityScheme{
					"auth": {Type: "http", Scheme: "bearer"},
				}
			}
		}
	}
	return nil
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
	// additionalProperties is false because the generated decoder says so:
	// transport.tmpl decodes request bodies with StrictJSONDecodeOptions, which
	// sets DisallowUnknownFields, so an unknown property is a 400. A document
	// that stays silent means "extra properties are allowed", and a client
	// generated from it sends one and gets rejected by the service the same
	// generator wrote. On a response schema it is the same statement from the
	// other side: these properties are what the struct marshals, and nothing else.
	schema := &openAPISchema{Type: "object", Properties: map[string]*openAPISchema{}, AdditionalProperties: false}
	if message == nil {
		return schema
	}
	schema.Description = message.Description
	for _, field := range message.Fields {
		if field == nil || field.JSONName == "-" {
			continue
		}
		name := FirstNonEmpty(field.JSONName, strings.ToLower(field.Name))
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
	if IsPointerGoType(field.GoType) {
		schema = nullableSchema(schema)
	}
	schema.Description = field.Description
	if field.Example != "" {
		var example any
		if err := json.Unmarshal([]byte(field.Example), &example); err != nil {
			example = strings.Trim(field.Example, `"`)
		}
		schema.Example = example
	}
	applyConstraints(schema, field)
	return schema
}

// applyConstraints carries the IDL's validate tag onto the contract as JSON
// Schema keywords, so a form generated from the document validates with the
// same rules the service enforces. One tag, one rule, three readers: enum
// bounds a string or number to its listed values (the dropdown's options),
// min and max bound a number, minLen and maxLen bound a string's length, and
// default is the value a form pre-fills — a hint the contract carries, never
// a rule anything enforces.
func applyConstraints(schema *openAPISchema, field *ir.Field) {
	cons := field.Constraints
	if cons.Empty() {
		return
	}
	if len(cons.Enum) > 0 {
		values := make([]any, 0, len(cons.Enum))
		for _, entry := range cons.Enum {
			values = append(values, typedScalar(entry, field.GoType))
		}
		schema.Enum = values
	}
	if cons.Min != nil && isNumericGoType(field.GoType) {
		schema.Minimum = cons.Min
	}
	if cons.Max != nil && isNumericGoType(field.GoType) {
		schema.Maximum = cons.Max
	}
	if isTextualGoType(field.GoType) {
		if cons.MinLen != nil {
			schema.MinLength = cons.MinLen
		}
		if cons.MaxLen != nil {
			schema.MaxLength = cons.MaxLen
		}
	}
	if cons.HasDefault {
		schema.Default = typedScalar(cons.Default, field.GoType)
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

// typedScalar converts one validate-tag value into the JSON type of the field
// it belongs to, so the contract says `"enum": ["active"]` for a string field
// and `"enum": [1, 2]` for an int field — not a document where every value is
// a string.
func typedScalar(value, goType string) any {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(goType), "*"))
	if isNumericGoType(t) {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	if t == "bool" {
		return value == "true"
	}
	return value
}

func IsPointerGoType(goType string) bool {
	return strings.HasPrefix(strings.TrimSpace(goType), "*")
}

// nullableSchema restates a schema so that it also admits null.
//
// A generated struct carries no `omitempty` — model.tmpl writes `json:"name"` —
// so a nil pointer field marshals to `null`, and a schema naming only the value's
// type describes a payload the service does not produce. A client generated from
// it fails to unmarshal a response the service considers valid.
//
// The spelling depends on what is being made nullable. A plain type becomes a
// two-element type list, which is how JSON Schema 2020-12 says it. A $ref cannot:
// keywords beside a $ref were ignored before 2020-12 and validators still differ,
// so the reference is wrapped in anyOf where every implementation agrees.
func nullableSchema(schema *openAPISchema) *openAPISchema {
	if schema == nil {
		return nil
	}
	if schema.Ref != "" {
		return &openAPISchema{AnyOf: []*openAPISchema{{Ref: schema.Ref}, {Type: "null"}}}
	}
	if name, ok := schema.Type.(string); ok && name != "" && name != "null" {
		schema.Type = []string{name, "null"}
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
		return &openAPISchema{Type: FirstNonEmpty(schemaType, "object")}
	}
	return &openAPISchema{Type: FirstNonEmpty(schemaType, "object")}
}

func openAPIRef(name string) *openAPISchema {
	return contractSchemaRef(openAPIRefPrefix, name)
}

func contractSchemaRef(refPrefix, name string) *openAPISchema {
	return &openAPISchema{Ref: refPrefix + name}
}

func ContractMessageNames(messages map[string]*ir.Message) map[string]struct{} {
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

// errorResponseSchema describes the error envelope the framework's JSON error
// encoder writes.
//
// It deliberately does not set additionalProperties: false, unlike the message
// schemas. The body is not produced by generated code — the encoder is a
// deployment's choice, and ProblemJSONErrorEncoder answers with a wider document
// — so closing this schema would make the generator promise something it does not
// write.
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

func CollectOpenAPIMessages(project *ir.Project) map[string]*ir.Message {
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
				if _, ok := messages[method.Input.Name]; !ok {
					messages[method.Input.Name] = method.Input
				}
			}
			if method.Output != nil && method.Output.Name != "" {
				if _, ok := messages[method.Output.Name]; !ok {
					messages[method.Output.Name] = method.Output
				}
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
		return FirstNonEmpty(project.Services[0].Title, project.Services[0].Name+" API")
	}
	return FirstNonEmpty(project.PackageName, "Microservice") + " API"
}

func openAPIDescription(project *ir.Project) string {
	if len(project.Services) == 1 && project.Services[0] != nil {
		return FirstNonEmpty(project.Services[0].Description, project.Services[0].Name+" service API")
	}
	return "Generated service API"
}

func JoinOpenAPIPath(prefix, route string) string {
	path := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(route, "/")
	if path == "" {
		return "/"
	}
	return path
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
