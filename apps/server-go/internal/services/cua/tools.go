// Harness tool wrappers over the Cua Driver inventory.
package cua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// cuaTool adapts one driver tool to the harness Tool interface.
type cuaTool struct {
	manager *Manager
	driver  *Driver
	def     ToolDef
	name    string
	tier    tools.ToolTier
}

// NewTool wraps one advertised Cua tool.
func NewTool(manager *Manager, driver *Driver, def ToolDef) tools.Tool {
	return &cuaTool{
		manager: manager,
		driver:  driver,
		def:     def,
		name:    ToolName(def.Name),
		tier:    Tier(def),
	}
}

func (t *cuaTool) Name() string { return t.name }

func (t *cuaTool) Description() string {
	desc := t.def.Description
	if desc == "" {
		desc = "Cua Driver tool " + t.def.Name
	}
	// Carry the driver's own risk labels so the model knows which operations
	// are gated and how strict the runtime is.
	var notes []string
	if class := t.def.RiskClass(); class != "" {
		notes = append(notes, "risk "+strings.ToUpper(class))
	}
	if t.def.Risk.OperationSensitive {
		notes = append(notes, "the exact risk depends on the arguments")
	}
	if len(notes) > 0 {
		desc += " (" + strings.Join(notes, ", ") + ")"
	}
	return desc
}

func (t *cuaTool) Tier() tools.ToolTier       { return t.tier }
func (t *cuaTool) Schema() *jsonschema.Schema { return &jsonschema.Schema{Type: "object"} }

// RawSchema hands the driver's own input schema to the provider, so argument
// names and types are exactly what Cua expects rather than a flattened guess.
func (t *cuaTool) RawSchema() map[string]any {
	if len(t.def.InputSchema) == 0 {
		return map[string]any{"type": "object"}
	}
	var schema map[string]any
	if err := json.Unmarshal(t.def.InputSchema, &schema); err != nil {
		return map[string]any{"type": "object"}
	}
	// A schema without a type is rejected by some providers; the driver's
	// tools are all object-shaped, so default it.
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	return schema
}

func (t *cuaTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	args := map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, tools.NewToolError("Invalid arguments for %s: %v", t.name, err)
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	result, err := t.driver.Call(callCtx, t.def.Name, args, t.manager.Stopped)
	if err != nil {
		// A cancelled call must say its outcome is unknown: the action may
		// already have taken effect on screen. Retrying blindly could click
		// the same button twice or send a message again.
		var cancelled *CancelledError
		if errorsAs(err, &cancelled) {
			return nil, tools.NewToolError("%s stopped: %s", t.def.Name, cancelled.Error())
		}
		return nil, tools.NewToolError("Cua Driver tool %s failed: %v", t.def.Name, err)
	}

	parts := ToolResultParts(result)
	return tools.Envelope{Content: parts, IsError: result.IsError}, nil
}

// errorsAs is errors.As without importing errors at every call site; kept
// local so the cancellation branch reads cleanly.
func errorsAs(err error, target **CancelledError) bool {
	for err != nil {
		if c, ok := err.(*CancelledError); ok {
			*target = c
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// Loader is what /computer-use invokes to bring the tools into a run. It is
// separate from Manager so the group can be loaded lazily, and so a run that
// never invoked the command never sees a computer-use tool at all.
type Loader struct {
	manager *Manager
	mu      sync.Mutex
	loaded  bool
}

// NewLoader returns a loader bound to manager.
func NewLoader(manager *Manager) *Loader { return &Loader{manager: manager} }

// Loaded reports whether the tools have been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends every driver tool to registry and marks the group loaded.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	built, err := l.manager.Tools()
	if err != nil {
		return 0, &ErrNoDriver{Cause: err}
	}
	registry.Add(built...)
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return len(built), nil
}

// Describe returns a short human summary of the tool surface, for the skill
// text and for a status endpoint.
func (l *Loader) Describe() (string, error) {
	driver, err := l.manager.Driver()
	if err != nil {
		return "", &ErrNoDriver{Cause: err}
	}
	inventory, err := driver.ListTools()
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(inventory))
	for _, def := range inventory {
		names = append(names, def.Name)
	}
	return fmt.Sprintf("%d tools: %s", len(names), strings.Join(names, ", ")), nil
}
