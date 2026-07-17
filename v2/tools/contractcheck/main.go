package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pb33f/libopenapi"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func main() {
	openAPIPath := flag.String("openapi", "", "path to an OpenAPI 3.1 document")
	schemaPath := flag.String("schema", "", "path to a JSON Schema 2020-12 document")
	flag.Parse()
	if *openAPIPath == "" || *schemaPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run . -openapi <openapi.json> -schema <schema.json>")
		os.Exit(2)
	}

	if err := errors.Join(validateOpenAPI(*openAPIPath), validateJSONSchema(*schemaPath)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("validated OpenAPI 3.1 and JSON Schema 2020-12 contracts in %s\n", *openAPIPath)
}

func validateOpenAPI(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OpenAPI %q: %w", path, err)
	}
	document, err := libopenapi.NewDocument(data)
	if err != nil {
		return fmt.Errorf("parse OpenAPI %q: %w", path, err)
	}
	model, err := document.BuildV3Model()
	if err != nil {
		return fmt.Errorf("build OpenAPI model %q: %w", path, err)
	}
	if model == nil || model.Model.Version != "3.1.0" {
		var version string
		if model != nil {
			version = model.Model.Version
		}
		return fmt.Errorf("OpenAPI version = %q, want 3.1.0", version)
	}
	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil || model.Model.Paths.PathItems.Len() == 0 {
		return fmt.Errorf("OpenAPI %q has no paths", path)
	}
	if model.Model.Components == nil || model.Model.Components.Schemas == nil || model.Model.Components.Schemas.Len() == 0 {
		return fmt.Errorf("OpenAPI %q has no component schemas", path)
	}
	return nil
}

func validateJSONSchema(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON Schema %q: %w", path, err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse JSON Schema %q: %w", path, err)
	}
	if got := document["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
		return fmt.Errorf("JSON Schema dialect = %v, want draft 2020-12", got)
	}
	definitions, ok := document["$defs"].(map[string]any)
	if !ok || len(definitions) == 0 {
		return fmt.Errorf("JSON Schema %q has no $defs", path)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const resource = "generated-schema.json"
	if err := compiler.AddResource(resource, document); err != nil {
		return fmt.Errorf("add JSON Schema resource %q: %w", path, err)
	}
	if _, err := compiler.Compile(resource); err != nil {
		return fmt.Errorf("compile JSON Schema %q: %w", path, err)
	}

	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pointer := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
		if _, err := compiler.Compile(resource + "#/$defs/" + pointer); err != nil {
			return fmt.Errorf("compile JSON Schema definition %q: %w", name, err)
		}
	}
	return nil
}
