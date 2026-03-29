// Package config provides local configuration loading from .local.env files.
// It never overrides environment variables that are already set.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Load reads key=value pairs from ~/.agora/local.env or ./local.env
// and sets them as environment variables (only if not already set).
// This allows local development config without committing secrets to git.
func Load() error {
	path := localEnvPath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("load local env %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return nil
}

// localEnvPath returns the path to the local env file, checking ~ first then CWD.
func localEnvPath() string {
	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".agora", "local.env"),
		".local.env",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
