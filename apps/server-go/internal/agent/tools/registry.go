// Tool registry: lookup + provider-facing definitions.
package tools

import (
	"fmt"
	"sort"
)

type Registry struct {
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, error) {
	t, ok := r.tools[name]
	if !ok {
		return nil, NewToolError("Unknown tool: %s", name)
	}
	return t, nil
}

func (r *Registry) Names() []string {
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

// Definitions renders every tool for provider tool lists (stable order).
func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.tools))
	for _, name := range r.Names() {
		t := r.tools[name]
		schema, err := SchemaMap(t)
		if err != nil {
			panic(fmt.Sprintf("tool %s: invalid schema: %v", name, err))
		}
		out = append(out, Definition{Name: t.Name(), Description: t.Description(), InputSchema: schema})
	}
	return out
}
