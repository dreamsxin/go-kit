package generator

import "github.com/dreamsxin/go-kit/v2/cmd/microgen/ir"

const (
	jsonSchemaVersion   = "https://json-schema.org/draft/2020-12/schema"
	jsonSchemaRefPrefix = "#/$defs/"
)

type jsonSchemaDocument struct {
	Schema      string                    `json:"$schema"`
	Title       string                    `json:"title"`
	Description string                    `json:"description,omitempty"`
	Defs        map[string]*openAPISchema `json:"$defs"`
}

func buildJSONSchemaDocument(project *ir.Project) jsonSchemaDocument {
	if project == nil {
		project = &ir.Project{}
	}
	messages := collectOpenAPIMessages(project)
	messageNames := contractMessageNames(messages)
	return jsonSchemaDocument{
		Schema:      jsonSchemaVersion,
		Title:       openAPITitle(project) + " Schemas",
		Description: openAPIDescription(project),
		Defs:        buildContractSchemas(messages, messageNames, jsonSchemaRefPrefix),
	}
}
