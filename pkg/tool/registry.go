package tool

import (
	"fmt"
	"sync"

	"github.com/smrobot988-design/Agora/pkg/schema"
)

// Registry holds registered tools, keyed by name.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Returns an error if a tool with the same name
// is already registered or the tool name is empty.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Definition().Name
	if name == "" {
		return fmt.Errorf("register tool: name must not be empty")
	}
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("register tool: %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// Get returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Definitions returns ToolDefinition for all registered tools.
func (r *Registry) Definitions() []schema.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]schema.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}
