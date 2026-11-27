package console

import (
	"context"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// ArgSpec documents one argument a console method accepts. It feeds argument
// validation, console.help and console.describe, so it is the only place an
// argument is described.
type ArgSpec struct {
	Type     string // "string", "integer", "boolean", "object" or "array"
	Required bool
	Doc      string
}

// methodDef is one console.* method: its model-facing contract plus the
// handler that serves it. The catalog in registry.go is the single source of
// truth — kernel registration, argument validation, help and describe all
// read from it, so a method cannot drift between those surfaces.
type methodDef struct {
	name               string
	description        string
	requiresFullAccess bool
	args               map[string]ArgSpec
	handler            func(*Runtime, context.Context, map[string]any) (*jskernel.Result, error)
}

// argNames lists the accepted argument names: required first, then optional.
// Map order is random, so sort for stable error messages and signatures.
func (d methodDef) argNames() []string {
	names := make([]string, 0, len(d.args))
	for name := range d.args {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := d.args[names[i]].Required, d.args[names[j]].Required
		if ri != rj {
			return ri
		}
		return names[i] < names[j]
	})
	return names
}

// signatureArgs renders the argument list for MethodSignatures:
// "({session_id, limit?, ...})" with required args first.
func (d methodDef) signatureArgs() string {
	if len(d.args) == 0 {
		return "()"
	}
	parts := make([]string, 0, len(d.args))
	for _, name := range d.argNames() {
		if d.args[name].Required {
			parts = append(parts, name)
		} else {
			parts = append(parts, name+"?")
		}
	}
	return "({" + strings.Join(parts, ", ") + "})"
}

// namespace is the grouping a method belongs to: the part before the first
// dot, or "core" for unqualified names like "models".
func (d methodDef) namespace() string {
	for i, c := range d.name {
		if c == '.' {
			return d.name[:i]
		}
	}
	return "core"
}
