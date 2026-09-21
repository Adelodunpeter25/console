// memory tool: persistent cross-session facts. Port of
// apps/server/agent/src/tools/memory.ts (store/recall/list/edit/delete
// over project/global scopes, keyword recall, tag rendering).
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
)

type memoryInput struct {
	Op      string   `json:"op" jsonschema:"required,description=Operation to apply: 'store', 'recall', 'list', 'edit', or 'delete'"`
	Content string   `json:"content,omitempty" jsonschema:"description=Memory content (required for 'store'; new content for 'edit')"`
	Tags    []string `json:"tags,omitempty" jsonschema:"description=Tags for filtering (used with 'store', 'edit', 'recall', or 'list')"`
	Scope   string   `json:"scope,omitempty" jsonschema:"description=Defaults to 'project'. Use 'global' only for facts that are not tied to this project's code"`
	Query   string   `json:"query,omitempty" jsonschema:"description=Free-text search term (used with 'recall')"`
	ID      string   `json:"id,omitempty" jsonschema:"description=Memory id (required for 'edit' and 'delete'; must match the entry's scope)"`
}

// NewMemoryTool builds the "memory" tool bound to a project and registry.
// A nil registry reports unavailable (the run service always wires one).
func NewMemoryTool(projectID string, registry *memory.Registry) Tool {
	return NewTool("memory",
		"Persistent memory across sessions. Store only what the user explicitly asks to remember; recall or list before assuming. Defaults to project scope.",
		TierWrite,
		func(ctx context.Context, in memoryInput) (any, error) {
			if registry == nil {
				return nil, NewToolError("Memory is not available in this run.")
			}
			scope := memory.ScopeProject
			if in.Scope != "" {
				scope = memory.Scope(in.Scope)
			}
			if scope != memory.ScopeProject && scope != memory.ScopeGlobal {
				return nil, NewToolError("Unknown scope '%s'.", in.Scope)
			}
			store, err := registry.Resolve(scope, projectID)
			if err != nil {
				return nil, NewToolError("%s", err.Error())
			}
			switch in.Op {
			case "store":
				if in.Content == "" {
					return nil, NewToolError("'store' operation requires 'content'.")
				}
				entry, err := store.Store(in.Content, in.Tags)
				if err != nil {
					return nil, err
				}
				return fmt.Sprintf("Stored memory %s (scope: %s).", entry.ID, entry.Scope), nil
			case "recall":
				if in.Query == "" && len(in.Tags) == 0 {
					return nil, NewToolError("'recall' operation requires 'query' and/or 'tags'.")
				}
				entries, err := store.List(nil)
				if err != nil {
					return nil, err
				}
				matches := memory.RecallMemories(entries, memory.Query{Text: in.Query, Tags: in.Tags}, 10)
				got := make([]memory.Entry, 0, len(matches))
				for _, m := range matches {
					got = append(got, m.Entry)
				}
				return renderMemories(fmt.Sprintf("Recalled memories (scope: %s)", scope), got), nil
			case "list":
				entries, err := store.List(in.Tags)
				if err != nil {
					return nil, err
				}
				return renderMemories(fmt.Sprintf("Memories (scope: %s)", scope), entries), nil
			case "edit":
				if in.ID == "" {
					return nil, NewToolError("'edit' operation requires 'id'.")
				}
				if in.Content == "" && in.Tags == nil {
					return nil, NewToolError("'edit' operation requires 'content' and/or 'tags' to update.")
				}
				var content *string
				if in.Content != "" {
					content = &in.Content
				}
				updated, err := store.Update(in.ID, content, in.Tags, in.Tags != nil)
				if err != nil {
					return nil, err
				}
				if updated == nil {
					return nil, NewToolError("Memory %s not found in scope '%s'.", in.ID, scope)
				}
				return fmt.Sprintf("Updated memory %s.", updated.ID), nil
			case "delete":
				if in.ID == "" {
					return nil, NewToolError("'delete' operation requires 'id'.")
				}
				removed, err := store.Remove(in.ID)
				if err != nil {
					return nil, err
				}
				if !removed {
					return nil, NewToolError("Memory %s not found in scope '%s'.", in.ID, scope)
				}
				return fmt.Sprintf("Deleted memory %s.", in.ID), nil
			default:
				return nil, NewToolError("Unknown operation.")
			}
		})
}

func renderMemories(title string, entries []memory.Entry) string {
	if len(entries) == 0 {
		return title + ":\n(No memories found)"
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		suffix := ""
		if len(e.Tags) > 0 {
			suffix = " [" + strings.Join(e.Tags, ", ") + "]"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s%s", e.ID, e.Content, suffix))
	}
	return title + ":\n" + strings.Join(lines, "\n")
}
