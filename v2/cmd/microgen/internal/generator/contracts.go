package generator

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/internal/contract"
)

// generateContracts writes the reviewed OpenAPI and JSON Schema documents that
// describe the generated service. The documents themselves are built by
// internal/contract; this method owns where they live and how they are written.
func (g *Generator) generateContracts(ctx generationContext) error {
	if err := os.MkdirAll(g.layout.docsDir(), 0o755); err != nil {
		return err
	}
	if err := g.executeTemplate("docs.tmpl", g.layout.docsEmbed(), struct{}{}); err != nil {
		return err
	}
	doc := contract.BuildOpenAPIDocument(ctx.project, g.config.RoutePrefix)
	if err := writeJSONDocument(g.layout.openAPIFile(), doc); err != nil {
		return fmt.Errorf("write OpenAPI document: %w", err)
	}
	if err := writeJSONDocument(g.layout.jsonSchemaFile(), contract.BuildJSONSchemaDocument(ctx.project)); err != nil {
		return fmt.Errorf("write JSON Schema document: %w", err)
	}
	return nil
}

// writeJSONDocument writes one generated JSON document with the indentation and
// trailing newline the contract snapshots pin.
func writeJSONDocument(path string, document any) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
