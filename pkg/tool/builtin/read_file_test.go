package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileDefinition(t *testing.T) {
	r := NewReadFile()
	def := r.Definition()

	if def.Name != "read_file" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "read_file")
	}
	if def.Description == "" {
		t.Error("Definition().Description is empty")
	}
	found := false
	for _, req := range def.InputSchema.Required {
		if req == "path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Definition().InputSchema.Required does not contain 'path'")
	}
}

func TestReadFileBasic(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "line one") {
		t.Error("result does not contain 'line one'")
	}
	if !strings.Contains(result, "line two") {
		t.Error("result does not contain 'line two'")
	}
	if !strings.Contains(result, "line three") {
		t.Error("result does not contain 'line three'")
	}
}

func TestReadFileWithOffset(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString("line ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"path":   path,
		"offset": float64(5),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "line 5") {
		t.Errorf("result does not contain 'line 5', got: %s", result)
	}
	if strings.Contains(result, "line 1") {
		t.Errorf("result should not contain 'line 1' (before offset), got: %s", result)
	}
}

func TestReadFileWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString("line ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"path":  path,
		"limit": float64(3),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	count := strings.Count(result, "line ")
	if count != 3 {
		t.Errorf("result contains %d lines, want 3", count)
	}
}

func TestReadFileWithOffsetAndLimit(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString("line ")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"path":   path,
		"offset": float64(3),
		"limit":  float64(2),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	count := strings.Count(result, "line ")
	if count != 2 {
		t.Errorf("result contains %d lines, want 2", count)
	}
	if !strings.Contains(result, "line 3") {
		t.Errorf("result does not contain 'line 3', got: %s", result)
	}
	if !strings.Contains(result, "line 4") {
		t.Errorf("result does not contain 'line 4', got: %s", result)
	}
}

func TestReadFileNotFound(t *testing.T) {
	r := NewReadFile()
	_, err := r.Execute(context.Background(), map[string]interface{}{"path": "/nonexistent/file.txt"})
	if err == nil {
		t.Error("Execute expected error for nonexistent file, got nil")
	}
}

func TestReadFileIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	r := NewReadFile()
	_, err := r.Execute(context.Background(), map[string]interface{}{"path": tmpDir})
	if err == nil {
		t.Error("Execute expected error for directory, got nil")
	}
}

func TestReadFileEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{"path": path})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("result = %q, want empty string", result)
	}
}

func TestReadFileMissingPath(t *testing.T) {
	r := NewReadFile()
	_, err := r.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("Execute expected error for missing path, got nil")
	}
}

func TestReadFileOffsetBeyondFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(path, []byte("line one\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	r := NewReadFile()
	result, err := r.Execute(context.Background(), map[string]interface{}{
		"path":   path,
		"offset": float64(100),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "" {
		t.Errorf("result = %q, want empty string for offset beyond file", result)
	}
}
