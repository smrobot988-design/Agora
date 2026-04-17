package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

func newToolCall(id, name, rawArguments string) schema.ToolCall {
	call := schema.ToolCall{
		ID:           id,
		Name:         name,
		RawArguments: rawArguments,
	}
	input, parseError := parseToolArguments(rawArguments)
	call.Input = input
	call.ParseError = parseError
	return call
}

// NewToolCallFromRaw constructs a ToolCall from provider raw JSON arguments while
// preserving parse diagnostics for later validation and recovery.
func NewToolCallFromRaw(id, name, rawArguments string) schema.ToolCall {
	return newToolCall(id, name, rawArguments)
}

func parseToolArguments(raw string) (map[string]interface{}, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]interface{}{}, ""
	}

	seen := make(map[string]struct{})
	for _, candidate := range toolArgumentCandidates(trimmed) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			if parsed == nil {
				return map[string]interface{}{}, ""
			}
			return parsed, ""
		}
	}

	return nil, fmt.Sprintf("invalid JSON arguments: %s", trimmed)
}

func toolArgumentCandidates(raw string) []string {
	candidates := []string{raw}
	stripped := stripMarkdownCodeFence(raw)
	if stripped != raw {
		candidates = append(candidates, stripped)
	}

	extracted := extractJSONObject(raw)
	if extracted != "" && extracted != raw {
		candidates = append(candidates, extracted)
	}
	if stripped != "" {
		extractedStripped := extractJSONObject(stripped)
		if extractedStripped != "" && extractedStripped != stripped {
			candidates = append(candidates, extractedStripped)
		}
	}

	base := append([]string(nil), candidates...)
	for _, candidate := range base {
		withoutTrailingCommas := removeTrailingCommas(candidate)
		if withoutTrailingCommas != candidate {
			candidates = append(candidates, withoutTrailingCommas)
		}
		closed := closeJSONDelimiters(candidate)
		if closed != candidate {
			candidates = append(candidates, closed)
		}
		closedNoTrailing := closeJSONDelimiters(withoutTrailingCommas)
		if closedNoTrailing != withoutTrailingCommas {
			candidates = append(candidates, closedNoTrailing)
		}
	}

	return candidates
}

func stripMarkdownCodeFence(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") {
		return raw
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) < 2 {
		return raw
	}
	lines = lines[1:]
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1]
			}
		}
	}
	return raw[start:]
}

func removeTrailingCommas(raw string) string {
	var builder strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			builder.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			builder.WriteByte(ch)
			continue
		}
		if ch == ',' {
			next := nextNonSpaceByte(raw, i+1)
			if next == '}' || next == ']' {
				continue
			}
		}
		builder.WriteByte(ch)
	}

	return builder.String()
}

func closeJSONDelimiters(raw string) string {
	var stack []byte
	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == ch {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if len(stack) == 0 {
		return raw
	}

	var builder strings.Builder
	builder.WriteString(raw)
	for i := len(stack) - 1; i >= 0; i-- {
		builder.WriteByte(stack[i])
	}
	return builder.String()
}

func nextNonSpaceByte(raw string, index int) byte {
	for i := index; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return raw[i]
		}
	}
	return 0
}
