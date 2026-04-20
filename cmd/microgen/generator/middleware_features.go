package generator

import (
	"fmt"
	"slices"
	"strings"
)

var supportedGeneratedMiddlewares = []string{
	"tracing",
	"error-handling",
	"metrics",
}

func normalizeGeneratedMiddlewares(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		name := strings.TrimSpace(strings.ToLower(item))
		switch name {
		case "", "none":
			continue
		case "error", "errorhandling", "error_handling":
			name = "error-handling"
		}
		if !slices.Contains(supportedGeneratedMiddlewares, name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func validateGeneratedMiddlewareNames(items []string) error {
	normalized := normalizeGeneratedMiddlewares(items)
	if len(normalized) != len(compactGeneratedMiddlewareInputs(items)) {
		return fmt.Errorf("unsupported generated middleware name; supported values: %s", strings.Join(supportedGeneratedMiddlewares, ", "))
	}
	return nil
}

func compactGeneratedMiddlewareInputs(items []string) []string {
	var out []string
	for _, item := range items {
		name := strings.TrimSpace(strings.ToLower(item))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
