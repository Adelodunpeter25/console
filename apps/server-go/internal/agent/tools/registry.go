// Tool registry: lookup + provider-facing definitions.
package tools

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the tools available to one run. Tools may be added
// mid-run (lazy groups such as MCP servers); added tools are listed after
// the base set in insertion order so the earlier tool-list prefix stays
// byte-identical and the provider's prompt cache is not disturbed.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	added []string
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

// Add registers tools after construction. Existing names are left untouched.
func (r *Registry) Add(tools ...Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tools {
		if _, exists := r.tools[t.Name()]; exists {
			continue
		}
		r.tools[t.Name()] = t
		r.added = append(r.added, t.Name())
	}
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, NewToolError("Unknown tool: %s", name)
	}
	return t, nil
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Definition is the provider-facing tool descriptor.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Definitions renders every tool for provider tool lists: base tools in
// name order, then lazily added tools in the order they were added.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	isAdded := make(map[string]bool, len(r.added))
	for _, name := range r.added {
		isAdded[name] = true
	}
	base := make([]string, 0, len(r.tools))
	for name := range r.tools {
		if !isAdded[name] {
			base = append(base, name)
		}
	}
	sort.Strings(base)
	out := make([]Definition, 0, len(r.tools))
	for _, name := range append(base, r.added...) {
		t := r.tools[name]
		schema, err := SchemaMap(t)
		if err != nil {
			panic(fmt.Sprintf("tool %s: invalid schema: %v", name, err))
		}
		out = append(out, Definition{Name: t.Name(), Description: t.Description(), InputSchema: schema})
	}
	return out
}
