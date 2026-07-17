package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateOpenAPI(t *testing.T) {
	path := writeTestContract(t, "openapi.json", `{
  "openapi": "3.1.0",
  "info": {"title": "test", "version": "1.0.0"},
  "paths": {
    "/ping": {
      "get": {
        "responses": {"200": {"description": "ok"}}
      }
    }
  },
  "components": {
    "schemas": {"Ping": {"type": "object"}}
  }
}`)
	if err := validateOpenAPI(path); err != nil {
		t.Fatalf("validateOpenAPI: %v", err)
	}
}

func TestValidateOpenAPIRejectsWrongVersion(t *testing.T) {
	path := writeTestContract(t, "openapi.json", `{
  "openapi": "3.0.3",
  "info": {"title": "test", "version": "1.0.0"},
  "paths": {"/ping": {"get": {"responses": {"200": {"description": "ok"}}}}},
  "components": {"schemas": {"Ping": {"type": "object"}}}
}`)
	if err := validateOpenAPI(path); err == nil {
		t.Fatal("validateOpenAPI accepted OpenAPI 3.0")
	}
}

func TestValidateJSONSchema(t *testing.T) {
	path := writeTestContract(t, "schema.json", `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$defs": {
    "Thing": {
      "type": "object",
      "properties": {"id": {"type": "string"}}
    }
  }
}`)
	if err := validateJSONSchema(path); err != nil {
		t.Fatalf("validateJSONSchema: %v", err)
	}
}

func TestValidateJSONSchemaRejectsBrokenReference(t *testing.T) {
	path := writeTestContract(t, "schema.json", `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$defs": {
    "Thing": {"$ref": "#/$defs/Missing"}
  }
}`)
	if err := validateJSONSchema(path); err == nil {
		t.Fatal("validateJSONSchema accepted a missing definition reference")
	}
}

func writeTestContract(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}
