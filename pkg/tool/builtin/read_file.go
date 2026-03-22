package builtin

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// ReadFile reads the content of a file from the filesystem.
type ReadFile struct{}

// NewReadFile creates a ReadFile tool.
func NewReadFile() *ReadFile {
	return &ReadFile{}
}

// Definition returns the tool definition for the LLM.
func (r *ReadFile) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file content with line numbers. Use offset and limit to read specific portions of large files.",
		InputSchema: schema.PropertySchema{
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The absolute or relative path to the file to read",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "The line number to start reading from (1-based). Defaults to 1.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of lines to read. Defaults to reading the entire file.",
				},
			},
			Required: []string{"path"},
		},
	}
}

// Execute reads the file and returns its content with line numbers.
func (r *ReadFile) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	path, ok := stringFromInput(input, "path")
	if !ok || path == "" {
		return "", fmt.Errorf("read file: path is required")
	}

	offset := intFromInput(input, "offset", 1)
	limit := intFromInput(input, "limit", 0) // 0 means read all

	if offset < 1 {
		offset = 1
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("read file: file not found: %w", err)
		}
		return "", fmt.Errorf("read file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("read file: path is a directory, not a file")
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < offset {
			continue
		}
		if limit > 0 && len(lines) >= limit {
			break
		}
		lines = append(lines, fmt.Sprintf("%5d\t| %s", lineNum, scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read file: error reading file: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}
