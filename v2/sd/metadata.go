package sd

import (
	"fmt"
	"strconv"
	"strings"
)

// Registries hand labels over as strings — Consul Meta, Kubernetes labels, and
// Nomad meta are all map[string]string — while in-process callers naturally
// write typed literals. These readers coerce both, so a predicate written
// against int does not silently fail to match the "10" a registry delivered.

// MetadataString reads key as a string. Non-string values are rendered with
// their default format, which keeps registry-sourced and in-process values
// comparable.
func MetadataString(metadata map[string]any, key string) (string, bool) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return "", false
	}
	if text, isText := value.(string); isText {
		return text, true
	}
	return fmt.Sprint(value), true
}

// MetadataInt reads key as an int, accepting the integer, float, and string
// forms a registry or a caller may have stored.
func MetadataInt(metadata map[string]any, key string) (int, bool) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return number, true
	default:
		return 0, false
	}
}

// MetadataBool reads key as a bool, accepting the strings strconv.ParseBool
// understands so a registry can carry "true" or "1".
func MetadataBool(metadata map[string]any, key string) (bool, bool) {
	value, ok := metadata[key]
	if !ok || value == nil {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}
