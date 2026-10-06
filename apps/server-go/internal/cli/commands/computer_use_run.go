// The `console computer-use run` command: a scripted driver runner.
//
// This exists so computer-use work can be iterated without rebuilding the
// bundle binary. Every rebuild changes the ad-hoc signature and orphans the
// macOS TCC grant, so the binary inside the bundle stays frozen while testing
// and tasks are expressed as JSON scripts instead of new Go code.
//
// A script is an array of steps:
//
//	[
//	  {"label": "launch", "tool": "launch_app",
//	   "args": {"bundle_id": "com.apple.calculator"}, "as": "launch"},
//	  {"label": "wait", "tool": "$wait_window",
//	   "args": {"pid": "{{launch.pid}}", "timeout_ms": 15000}, "as": "win"},
//	  {"label": "read", "tool": "get_window_state",
//	   "args": {"pid": "{{launch.pid}}", "window_id": "{{win.window_id}}",
//	            "query": "seven"}, "as": "keys"},
//	  {"label": "pick seven", "tool": "$find",
//	   "args": {"from": "keys", "path": "elements",
//	            "match": {"label": "seven"}}, "as": "seven"},
//	  {"label": "press seven", "tool": "click",
//	   "args": {"pid": "{{launch.pid}}", "window_id": "{{win.window_id}}",
//	            "element_token": "{{seven.element_token}}",
//	            "delivery_mode": "background"}}
//	]
//
// Steps whose tool starts with "$" are interpreted locally rather than sent to
// the driver:
//
//   - $wait_window {pid, timeout_ms?, title_contains?} polls list_windows and
//     captures the first matching window object.
//   - $find {from, path?, match} picks one element out of a captured array by
//     matching fields, and captures it. A miss fails the step and lists the
//     labels that were actually present, because a wrong label is the usual bug.
//   - $sleep {ms} waits.
//
// "{{name.a.0.b}}" in any argument is replaced from previously captured
// values. A reference that is the whole string keeps its type, so window ids
// stay numbers; embedded references become text.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// scriptStep is one executable step. TimeoutMs bounds driver calls; the
// default keeps a wedged call from pinning the transcript forever.
type scriptStep struct {
	Label     string         `json:"label"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	As        string         `json:"as"`
	TimeoutMs int            `json:"timeout_ms"`
}

// scriptScope holds captured values by name: each step's "as" stores its
// structured result for later steps to reference.
type scriptScope map[string]any

var (
	scriptRefWhole = regexp.MustCompile(`^\{\{([^{}]+)\}\}$`)
	scriptRefAny   = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
)

// ResolveScriptPath walks a captured value by dotted path: map keys by name,
// array elements by index. The first segment is the capture name. Exported so
// the behaviour is pinned by tests rather than trusted.
func ResolveScriptPath(scope map[string]any, path string) (any, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("empty reference")
	}
	current, ok := scope[parts[0]]
	if !ok {
		return nil, fmt.Errorf("no captured value named %q", parts[0])
	}
	for _, part := range parts[1:] {
		switch node := current.(type) {
		case map[string]any:
			current, ok = node[part]
			if !ok {
				return nil, fmt.Errorf("%q has no field %q", strings.Join(parts, "."), part)
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, fmt.Errorf("%q: index %q out of range (length %d)",
					strings.Join(parts, "."), part, len(node))
			}
			current = node[index]
		default:
			return nil, fmt.Errorf("%q: cannot descend into %T", strings.Join(parts, "."), current)
		}
	}
	return current, nil
}

// stringifyScriptValue renders a captured value for embedding inside a larger
// string. Numbers that are whole stay plain; anything composite is JSON.
func stringifyScriptValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(raw)
	}
}

// InterpolateScriptValue replaces {{references}} inside one argument value,
// recursing through objects and arrays. A string that is exactly one reference
// keeps the referenced type; anything else becomes text.
func InterpolateScriptValue(value any, scope map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		if m := scriptRefWhole.FindStringSubmatch(v); m != nil {
			return ResolveScriptPath(scope, strings.TrimSpace(m[1]))
		}
		var firstErr error
		out := scriptRefAny.ReplaceAllStringFunc(v, func(match string) string {
			inner := scriptRefAny.FindStringSubmatch(match)[1]
			resolved, err := ResolveScriptPath(scope, strings.TrimSpace(inner))
			if err != nil {
				firstErr = err
				return match
			}
			return stringifyScriptValue(resolved)
		})
		return out, firstErr
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			resolved, err := InterpolateScriptValue(item, scope)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(v))
		for i, item := range v {
			resolved, err := InterpolateScriptValue(item, scope)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, resolved)
		}
		return out, nil
	default:
		return value, nil
	}
}

// FindScriptElement returns the first element whose fields all match, for
// example {"label": "seven"}. A miss is an error that lists the labels that
// were actually there, because a stale or mistyped label is the usual cause.
func FindScriptElement(elements []any, match map[string]any) (map[string]any, error) {
	if len(match) == 0 {
		return nil, fmt.Errorf("$find needs a non-empty match object")
	}
	var labels []string
	for _, item := range elements {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if label, ok := element["label"].(string); ok {
			labels = append(labels, label)
		}
		hit := true
		for key, want := range match {
			got, ok := element[key]
			if !ok || fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", want) {
				hit = false
				break
			}
		}
		if hit {
			return element, nil
		}
	}
	if len(labels) > 20 {
		labels = append(labels[:20], fmt.Sprintf("… and %d more", len(elements)-20))
	}
	return nil, fmt.Errorf("no element matching %v; present labels: %s", match, strings.Join(labels, ", "))
}

// transcriptImage is one image part reduced to what fits in a transcript: the
// multi-megabyte base64 never lands in the file, only its size.
type transcriptImage struct {
	MIMEType string `json:"mimeType"`
	Bytes    int    `json:"bytes"`
}

// transcriptEntry is one JSONL line of the run transcript.
type transcriptEntry struct {
	Index         int               `json:"i"`
	Label         string            `json:"label,omitempty"`
	Tool          string            `json:"tool"`
	Ms            int64             `json:"ms"`
	OK            bool              `json:"ok"`
	Error         string            `json:"error,omitempty"`
	IsError       *bool             `json:"isError,omitempty"`
	Text          string            `json:"text,omitempty"`
	TextTruncated bool              `json:"textTruncated,omitempty"`
	Images        []transcriptImage `json:"images,omitempty"`
	Structured    json.RawMessage   `json:"structured,omitempty"`
	Captured      string            `json:"captured,omitempty"`
}

func computerUseRunCmd() *cobra.Command {
	var scriptPath, outPath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute a JSON script of driver calls and write a JSONL transcript",
		Long: `Execute a JSON script of driver calls and write a JSONL transcript.

Each step calls one driver tool (or a local $ helper: $wait_window, $find,
$sleep), captures its structured result under "as", and later steps reference
it as {{name.path}}. The transcript records every step so a task can be
debugged without rebuilding anything.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if scriptPath == "" {
				return fmt.Errorf("pass --script with the path to a JSON step array")
			}
			cua.EnsureHostEnv()

			raw, err := os.ReadFile(scriptPath)
			if err != nil {
				return err
			}
			var steps []scriptStep
			if err := json.Unmarshal(raw, &steps); err != nil {
				return fmt.Errorf("script is not a JSON step array: %w", err)
			}
			if len(steps) == 0 {
				return fmt.Errorf("script has no steps")
			}

			var transcript *os.File
			if outPath != "" {
				transcript, err = os.Create(outPath)
				if err != nil {
					return err
				}
				defer transcript.Close()
			}

			driver, err := cua.Open(cua.Options{})
			if err != nil {
				return err
			}
			defer driver.Shutdown()

			scope := scriptScope{}
			failed := 0
			for i, step := range steps {
				entry := executeScriptStep(driver, scope, i, step)
				if transcript != nil {
					line, _ := json.Marshal(entry)
					fmt.Fprintln(transcript, string(line))
				}
				fmt.Printf("[%d/%d] %-28s %-18s %dms %s\n",
					i+1, len(steps), step.Label, step.Tool, entry.Ms, stepVerdict(entry))
				if !entry.OK {
					failed++
					if entry.Error != "" {
						fmt.Printf("        %s\n", firstLine(entry.Error))
					}
				}
			}
			fmt.Printf("\ndone: %d ok, %d failed\n", len(steps)-failed, failed)
			if failed > 0 {
				return fmt.Errorf("%d of %d steps failed", failed, len(steps))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scriptPath, "script", "", "Path to a JSON array of steps")
	cmd.Flags().StringVar(&outPath, "out", "", "Write the JSONL transcript here (needed when launched via open)")
	return cmd
}

func stepVerdict(entry transcriptEntry) string {
	if entry.OK {
		if len(entry.Images) > 0 {
			return fmt.Sprintf("ok (%d image(s))", len(entry.Images))
		}
		return "ok"
	}
	return "FAILED"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// executeScriptStep runs one step and returns its transcript entry. Local $
// helpers never reach the driver.
func executeScriptStep(driver *cua.Driver, scope scriptScope, index int, step scriptStep) transcriptEntry {
	entry := transcriptEntry{Index: index, Label: step.Label, Tool: step.Tool}
	start := time.Now()
	defer func() { entry.Ms = time.Since(start).Milliseconds() }()

	if step.Tool == "" {
		entry.Error = "step has no tool"
		return entry
	}

	interpolated, err := InterpolateScriptValue(map[string]any(step.Args), scope)
	if err != nil {
		entry.Error = fmt.Sprintf("bad reference: %v", err)
		return entry
	}
	args, _ := interpolated.(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	if strings.HasPrefix(step.Tool, "$") {
		return executeLocalStep(driver, step.Tool, args, step, scope, entry)
	}
	return executeDriverStep(driver, step, args, scope, entry)
}

// executeDriverStep calls one real driver tool with a deadline, so a wedged
// call fails the step instead of pinning the transcript forever.
func executeDriverStep(driver *cua.Driver, step scriptStep, args map[string]any, scope scriptScope, entry transcriptEntry) transcriptEntry {
	timeoutMs := step.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 120_000
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	result, err := driver.Call(ctx, step.Tool, args, nil)
	if err != nil {
		// Surface cancellations with the honest wording: an interrupted action
		// may already have taken effect, so the transcript must not suggest
		// the step simply did not happen.
		var cancelled *cua.CancelledError
		if errorsAsCua(err, &cancelled) {
			entry.Error = cancelled.Error()
			return entry
		}
		entry.Error = err.Error()
		return entry
	}

	isError := result.IsError
	entry.IsError = &isError
	for _, part := range result.Content {
		switch part.Type {
		case "text":
			if part.Text != "" {
				if entry.Text != "" {
					entry.Text += "\n"
				}
				entry.Text += part.Text
			}
		case "image":
			entry.Images = append(entry.Images, transcriptImage{MIMEType: part.MIMEType, Bytes: len(part.Data)})
		}
	}
	if len(entry.Text) > 2000 {
		entry.Text = entry.Text[:2000] + fmt.Sprintf("\n… [truncated, %d chars total]", len(entry.Text))
		entry.TextTruncated = true
	}
	if len(result.StructuredContent) > 0 {
		entry.Structured = result.StructuredContent
		var decoded any
		if json.Unmarshal(result.StructuredContent, &decoded) == nil && step.As != "" {
			scope[step.As] = decoded
			entry.Captured = step.As
		}
	}
	entry.OK = !result.IsError
	if result.IsError && entry.Error == "" {
		entry.Error = firstLine(entry.Text)
	}
	return entry
}

// executeLocalStep handles the $ helpers, which capture Go values rather than
// driver envelopes. They share the run's driver: Cua owns process-global
// executor threads, so a second handle in the same process risks a
// runtime-conflict rather than independence.
func executeLocalStep(driver *cua.Driver, tool string, args map[string]any, step scriptStep, scope scriptScope, entry transcriptEntry) transcriptEntry {
	switch tool {
	case "$sleep":
		ms, _ := args["ms"].(float64)
		if ms < 0 {
			ms = 0
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		entry.OK = true
		entry.Text = fmt.Sprintf("slept %dms", int(ms))
		return entry

	case "$wait_window":
		return executeWaitWindow(driver, args, step, scope, entry)

	case "$find":
		return executeFind(args, step, scope, entry)

	default:
		entry.Error = fmt.Sprintf("unknown local step %q (want $wait_window, $find or $sleep)", tool)
		return entry
	}
}

// executeWaitWindow polls list_windows because a freshly launched app reports
// window_ready=false: the window does not exist at the instant launch returns.
// The first window whose title contains title_contains (when given) is
// captured under "as".
func executeWaitWindow(driver *cua.Driver, args map[string]any, step scriptStep, scope scriptScope, entry transcriptEntry) transcriptEntry {
	pidValue, ok := args["pid"]
	if !ok {
		entry.Error = "$wait_window needs a pid"
		return entry
	}
	var pid int
	switch v := pidValue.(type) {
	case float64:
		pid = int(v)
	case int:
		pid = v
	default:
		entry.Error = fmt.Sprintf("$wait_window pid must be a number, got %T", pidValue)
		return entry
	}
	timeoutMs := 10_000
	if v, ok := args["timeout_ms"].(float64); ok && v > 0 {
		timeoutMs = int(v)
	}
	titleContains, _ := args["title_contains"].(string)

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		result, err := driver.Call(ctx, "list_windows", map[string]any{"pid": pid}, nil)
		cancel()
		if err != nil {
			entry.Error = err.Error()
			return entry
		}
		var payload struct {
			Windows []map[string]any `json:"windows"`
		}
		if len(result.StructuredContent) > 0 {
			_ = json.Unmarshal(result.StructuredContent, &payload)
		}
		for _, window := range payload.Windows {
			if titleContains != "" {
				title, _ := window["title"].(string)
				if !strings.Contains(title, titleContains) {
					continue
				}
			}
			name := step.As
			if name == "" {
				name = "window"
			}
			scope[name] = window
			entry.Captured = name
			entry.OK = true
			raw, _ := json.Marshal(window)
			entry.Structured = raw
			entry.Text = fmt.Sprintf("window %v on pid %d", window["window_id"], pid)
			return entry
		}
		time.Sleep(500 * time.Millisecond)
	}
	entry.Error = fmt.Sprintf("no window appeared on pid %d within %dms", pid, timeoutMs)
	return entry
}

// executeFind picks one element out of a captured array. The usual use is an
// accessibility snapshot's elements array matched by label.
func executeFind(args map[string]any, step scriptStep, scope scriptScope, entry transcriptEntry) transcriptEntry {
	from, _ := args["from"].(string)
	if from == "" {
		entry.Error = "$find needs a from capture name"
		return entry
	}
	base, ok := scope[from]
	if !ok {
		entry.Error = fmt.Sprintf("no captured value named %q", from)
		return entry
	}
	if path, _ := args["path"].(string); path != "" {
		resolved, err := ResolveScriptPath(scope, from+"."+path)
		if err != nil {
			entry.Error = err.Error()
			return entry
		}
		base = resolved
	}
	elements, ok := base.([]any)
	if !ok {
		entry.Error = fmt.Sprintf("$find needs an array, %q resolved to %T", from, base)
		return entry
	}
	match, _ := args["match"].(map[string]any)
	element, err := FindScriptElement(elements, match)
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	name := step.As
	if name == "" {
		name = "element"
	}
	scope[name] = element
	entry.Captured = name
	entry.OK = true
	raw, _ := json.Marshal(element)
	entry.Structured = raw
	entry.Text = fmt.Sprintf("matched element %q", element["label"])
	return entry
}

// errorsAsCua unwraps to a *cua.CancelledError without importing errors at
// every call site.
func errorsAsCua(err error, target **cua.CancelledError) bool {
	for err != nil {
		if c, ok := err.(*cua.CancelledError); ok {
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
