// Built-in lazy tool loading (harness plan Task 4.4, behind
// CONSOLE_HARNESS_LAZY_TOOLS): six rarely used tools are held back from the
// request and loaded on demand through `loadTools`, appended at the end of
// the tool list so the earlier, cached tool prefix never changes.
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// deferredToolNames are the built-ins held back when the lazy flag is on.
// `ask` stays deferred-never: models call it even when it is missing, so it
// must always be visible.
var deferredToolNames = []string{"webSearch", "webFetch", "bashJob", "askMany", "memory", "batchWrite"}

// holdBackDeferredTools splits the run's tool list into the always-sent part
// and the deferred tools, keeping both in order.
func holdBackDeferredTools(toolList []tools.Tool) (kept, held []tools.Tool) {
	deferred := make(map[string]bool, len(deferredToolNames))
	for _, name := range deferredToolNames {
		deferred[name] = true
	}
	for _, t := range toolList {
		if deferred[t.Name()] {
			held = append(held, t)
		} else {
			kept = append(kept, t)
		}
	}
	return kept, held
}

// lazyLoader is the combined `loadTools` tool: `names` loads deferred
// built-ins, `group` delegates to the MCP loader. One tool, because the
// registry cannot hold two tools with the same name.
type lazyLoader struct {
	registry *tools.Registry
	held     map[string]tools.Tool
	mcpTool  tools.Tool // standard MCP loadTools tool; nil when no servers
	onNames  func(names []string)
}

type lazyLoaderInput struct {
	Group string   `json:"group,omitempty" jsonschema:"description=External tool group to load, e.g. mcp:<server-id>."`
	Names []string `json:"names,omitempty" jsonschema:"description=Deferred built-in tools to load."`
}

// newLazyToolsLoader builds the combined loader. The returned *lazyLoader is
// the same instance the tool closes over; its registry field may be nil at
// construction and is wired up once the run's registry exists. mcpTool may
// be nil. onNames, when set, fires with every successfully requested name so
// the caller can persist loads across the session's runs.
func newLazyToolsLoader(registry *tools.Registry, held []tools.Tool, mcpTool tools.Tool, onNames func(names []string)) (tools.Tool, *lazyLoader) {
	l := &lazyLoader{registry: registry, held: make(map[string]tools.Tool, len(held)), mcpTool: mcpTool, onNames: onNames}
	for _, t := range held {
		l.held[t.Name()] = t
	}

	var b strings.Builder
	b.WriteString("Load deferred tools so they become callable on your next step; they stay available for the rest of the run. Pass `names` for built-in tools:")
	for _, name := range deferredToolNames {
		if t, ok := l.held[name]; ok {
			fmt.Fprintf(&b, "\n- %s: %s", name, firstSentence(t.Description()))
		}
	}
	if mcpTool != nil {
		b.WriteString("\n\n" + strings.TrimSpace(mcpTool.Description()))
	}

	return tools.NewTool("loadTools", b.String(), tools.TierRead,
		func(ctx context.Context, in lazyLoaderInput) (any, error) {
			if strings.TrimSpace(in.Group) != "" {
				if l.mcpTool == nil {
					return nil, tools.NewToolError("No external tool groups are configured. Pass `names` to load deferred tools.")
				}
				raw, err := json.Marshal(map[string]string{"group": in.Group})
				if err != nil {
					return nil, tools.NewToolError("%v", err)
				}
				return l.mcpTool.Execute(ctx, raw)
			}
			if len(in.Names) == 0 {
				return nil, tools.NewToolError("Pass `names` (built-in tools) or `group` (external tool group).")
			}
			var lines []string
			var loaded []string
			for _, name := range in.Names {
				t, ok := l.held[name]
				if !ok {
					return nil, tools.NewToolError("Unknown deferred tool %q. Available: %s", name, strings.Join(l.available(), ", "))
				}
				// Registry.Add skips names already present, so re-loading is
				// idempotent.
				l.registry.Add(t)
				loaded = append(loaded, name)
				lines = append(lines, fmt.Sprintf("- %s: %s", t.Name(), firstSentence(t.Description())))
			}
			if l.onNames != nil {
				l.onNames(loaded)
			}
			return []map[string]any{{"type": "text", "text": "Loaded. Callable from your next step:\n" + strings.Join(lines, "\n")}}, nil
		}), l
}

// ensure loads a held tool without a model request — the bashJob auto-load
// exception: when bash starts a background job (or one is promoted at
// timeout), bashJob must be visible even though the model never asked.
func (l *lazyLoader) ensure(name string) {
	if l == nil || l.registry == nil {
		return
	}
	if t, ok := l.held[name]; ok {
		l.registry.Add(t)
	}
}

func (l *lazyLoader) available() []string {
	out := make([]string, 0, len(l.held))
	for _, name := range deferredToolNames {
		if _, ok := l.held[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	return s
}

// lazyToolsSetupHint tells the model, in setup, that deferred tools exist.
func lazyToolsSetupHint(held []tools.Tool) string {
	names := make([]string, 0, len(held))
	for _, t := range held {
		names = append(names, t.Name())
	}
	return "# Deferred tools\nMore tools are available through the `loadTools` tool (pass `names`): " +
		strings.Join(names, ", ") + ". Load a tool before first use; it stays available for the rest of the run."
}

// bashJobAutoLoader wraps the bash tool so a started background job (explicit
// background=true, or a foreground command promoted at its timeout) makes
// bashJob loadable immediately — the model never sees a tool it was told to
// use before loadTools ran.
type bashJobAutoLoader struct {
	tools.Tool
	ensure func()
}

func (b *bashJobAutoLoader) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	out, err := b.Tool.Execute(ctx, arguments)
	if err == nil && b.startedBackground(arguments, out) {
		b.ensure()
	}
	return out, err
}

// startedBackground reports whether this call started (or promoted) a
// background job: either the model asked for background=true, or the result
// carries the promotion notice from the bash tool.
func (b *bashJobAutoLoader) startedBackground(arguments json.RawMessage, out any) bool {
	var in struct {
		Background bool `json:"background"`
	}
	if json.Unmarshal(arguments, &in) == nil && in.Background {
		return true
	}
	return strings.Contains(resultText(out), "moved to the background")
}

// resultText digs the text out of a tool result: either the bare
// []map[string]any content array or the {content, isError} envelope.
func resultText(out any) string {
	switch v := out.(type) {
	case []map[string]any:
		for _, part := range v {
			if s, ok := part["text"].(string); ok {
				return s
			}
		}
	case tools.Envelope:
		return resultText(v.Content)
	}
	return ""
}
