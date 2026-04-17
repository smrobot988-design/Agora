package schema

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// NormalizeToolInput validates and normalizes a tool input against the declared
// JSON schema subset used by Agora tools. Unknown fields are dropped.
func NormalizeToolInput(def ToolDefinition, input map[string]interface{}) (map[string]interface{}, []ValidationIssue) {
	if input == nil {
		input = map[string]interface{}{}
	}
	normalized, issues := normalizeObject(def.InputSchema.Properties, def.InputSchema.Required, input, "$")
	if len(issues) > 0 {
		return nil, issues
	}
	return normalized, nil
}

func normalizeObject(properties map[string]interface{}, required []string, input map[string]interface{}, path string) (map[string]interface{}, []ValidationIssue) {
	normalized := make(map[string]interface{})
	var issues []ValidationIssue

	for _, name := range required {
		if _, ok := input[name]; !ok {
			issues = append(issues, ValidationIssue{
				Path:     joinPath(path, name),
				Code:     "missing_required",
				Message:  fmt.Sprintf("missing required field %q", name),
				Expected: "present",
				Actual:   "missing",
			})
		}
	}

	for key, value := range input {
		schemaDef, ok := asSchemaMap(properties[key])
		if !ok {
			continue
		}
		normalizedValue, childIssues := normalizeValue(schemaDef, value, joinPath(path, key))
		if len(childIssues) > 0 {
			issues = append(issues, childIssues...)
			continue
		}
		normalized[key] = normalizedValue
	}

	if len(issues) > 0 {
		return nil, issues
	}
	return normalized, nil
}

func normalizeValue(def map[string]interface{}, value interface{}, path string) (interface{}, []ValidationIssue) {
	expectedType := resolveSchemaType(def)
	switch expectedType {
	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			return nil, invalidTypeIssue(path, expectedType, value)
		}
		properties, _ := asStringMap(def["properties"])
		required := asStringSlice(def["required"])
		normalized, issues := normalizeObject(properties, required, obj, path)
		if len(issues) > 0 {
			return nil, issues
		}
		if err := validateEnum(def, normalized, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return normalized, nil

	case "array":
		itemSchema, _ := asSchemaMap(def["items"])
		values, ok := value.([]interface{})
		if !ok {
			values = []interface{}{value}
		}
		normalized := make([]interface{}, 0, len(values))
		var issues []ValidationIssue
		for i, item := range values {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if len(itemSchema) == 0 {
				normalized = append(normalized, item)
				continue
			}
			normalizedItem, childIssues := normalizeValue(itemSchema, item, itemPath)
			if len(childIssues) > 0 {
				issues = append(issues, childIssues...)
				continue
			}
			normalized = append(normalized, normalizedItem)
		}
		if len(issues) > 0 {
			return nil, issues
		}
		if err := validateEnum(def, normalized, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return normalized, nil

	case "string":
		stringValue, ok := coerceString(value)
		if !ok {
			return nil, invalidTypeIssue(path, expectedType, value)
		}
		if err := validateEnum(def, stringValue, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return stringValue, nil

	case "integer":
		numberValue, ok := coerceInteger(value)
		if !ok {
			return nil, invalidTypeIssue(path, expectedType, value)
		}
		if err := validateEnum(def, numberValue, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return numberValue, nil

	case "number":
		numberValue, ok := coerceNumber(value)
		if !ok {
			return nil, invalidTypeIssue(path, expectedType, value)
		}
		if err := validateEnum(def, numberValue, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return numberValue, nil

	case "boolean":
		booleanValue, ok := coerceBoolean(value)
		if !ok {
			return nil, invalidTypeIssue(path, expectedType, value)
		}
		if err := validateEnum(def, booleanValue, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return booleanValue, nil

	default:
		if err := validateEnum(def, value, path); err != nil {
			return nil, []ValidationIssue{*err}
		}
		return value, nil
	}
}

func validateEnum(def map[string]interface{}, value interface{}, path string) *ValidationIssue {
	rawEnum, ok := def["enum"].([]interface{})
	if !ok || len(rawEnum) == 0 {
		return nil
	}
	for _, allowed := range rawEnum {
		if allowed == value {
			return nil
		}
	}
	return &ValidationIssue{
		Path:     path,
		Code:     "invalid_enum",
		Message:  fmt.Sprintf("value %v is not in enum", value),
		Expected: "enum",
		Actual:   fmt.Sprintf("%v", value),
	}
}

func invalidTypeIssue(path, expected string, value interface{}) []ValidationIssue {
	return []ValidationIssue{{
		Path:     path,
		Code:     "invalid_type",
		Message:  fmt.Sprintf("expected %s, got %s", expected, actualType(value)),
		Expected: expected,
		Actual:   actualType(value),
	}}
}

func resolveSchemaType(def map[string]interface{}) string {
	switch t := def["type"].(type) {
	case string:
		return t
	case []interface{}:
		for _, raw := range t {
			candidate, _ := raw.(string)
			if candidate != "" && candidate != "null" {
				return candidate
			}
		}
	}
	if _, ok := def["properties"]; ok {
		return "object"
	}
	if _, ok := def["items"]; ok {
		return "array"
	}
	return ""
}

func coerceString(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	case float64:
		if math.Mod(v, 1) == 0 {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return "", false
	}
}

func coerceInteger(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		if math.Mod(v, 1) == 0 {
			return v, true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return float64(parsed), true
		}
	}
	return 0, false
}

func coerceNumber(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func coerceBoolean(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	}
	return false, false
}

func actualType(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func asSchemaMap(value interface{}) (map[string]interface{}, bool) {
	return asStringMap(value)
}

func asStringMap(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if mapped, ok := value.(map[string]interface{}); ok {
		return mapped, true
	}
	return nil, false
}

func asStringSlice(value interface{}) []string {
	switch raw := value.(type) {
	case []string:
		return append([]string(nil), raw...)
	case []interface{}:
		result := make([]string, 0, len(raw))
		for _, item := range raw {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func joinPath(base, key string) string {
	if base == "" || base == "$" {
		return "$." + key
	}
	return base + "." + key
}
