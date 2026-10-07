// The computer-use JavaScript runtime: one persistent interpreter per chat
// session with a preinstalled `cua` object.
//
// Instead of registering every driver tool as a harness tool (56 schemas,
// ~28K tokens re-sent on every turn), the model gets two tools — `computer`
// to run code and `computer_reset` to clear state — and the whole driver API
// as callable methods documented once in the /computer-use skill. The
// per-turn schema cost stays flat no matter how many methods the driver
// grows; API knowledge is read once, not re-sent as definitions.
//
// goja is pure Go, so this builds everywhere the server builds, including a
// future Linux server, with no new native dependency.
package cua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// JSCaller is what the runtime drives: one tool call by name. Satisfied by
// *Driver; fakes satisfy it in tests so no native library is needed.
type JSCaller interface {
	Call(ctx context.Context, name string, args map[string]any, cancel func() bool) (*ToolResult, error)
	ListTools() ([]ToolDef, error)
}

// maxJSText caps the text pulled out of one evaluation, matching the result
// caps elsewhere so a 42K snapshot tree cannot blow up the context by itself.
// maxJSImages caps inline images for the same reason the MCP adapter caps
// them: screenshots are megabytes each.
const (
	maxJSCodeBytes = 200_000
	jsCallTimeout  = 5 * time.Minute
	// maxJSImages caps inline images for the same reason the MCP adapter caps
	// them: screenshots are megabytes each.
	maxJSImages = 4
)

// JSRuntime is one persistent JavaScript interpreter with a preinstalled
// `cua` object: one session's kernel, where variables and bindings survive
// between evaluations until reset. Exported so tests and the CLI can drive it
// without a chat session; the agent path goes through Manager, which owns
// one per session. goja runtimes are not safe for concurrent use, so every
// entry takes mu — including the interrupt path, which goja documents as
// safe to call from another goroutine.
type JSRuntime struct {
	mu     sync.Mutex
	rt     *goja.Runtime
	caller JSCaller
	output []string
	// evalCtx is the context of the evaluation currently running on the
	// Eval goroutine; cua methods read it without locking because only that
	// goroutine touches it.
	evalCtx context.Context
	// cancel is polled while a driver call is in flight, so a Stop
	// interrupts admitted work. Set by Manager, which wires its own stop
	// flag; nil means no external stop source.
	cancel    func() bool
	isPromise func(goja.Value, ...goja.Value) (goja.Value, error)
}

// newJSRuntime builds a runtime and installs print plus the cua object from
// the live inventory, so new driver methods appear with no Console release.
func NewJSRuntime(caller JSCaller) (*JSRuntime, error) {
	r := &JSRuntime{rt: goja.New(), caller: caller}
	r.rt.Set("print", r.print)
	defs, err := caller.ListTools()
	if err != nil {
		return nil, err
	}
	cua := r.rt.NewObject()
	for _, def := range defs {
		name := def.Name
		if name == "" {
			continue
		}
		if err := cua.Set(name, func(call goja.FunctionCall) goja.Value {
			return r.callTool(name, call)
		}); err != nil {
			return nil, fmt.Errorf("install cua.%s: %w", name, err)
		}
	}
	if err := r.rt.Set("cua", cua); err != nil {
		return nil, err
	}
	// Promise completion would be a silent no-op (no microtask pump runs
	// between evaluations), so detect it and fail loudly instead of letting
	// the model believe an action happened.
	helper, err := r.rt.RunString(`(function(v) { return v instanceof Promise; })`)
	if err != nil {
		return nil, err
	}
	isPromise, ok := goja.AssertFunction(helper)
	if !ok {
		return nil, fmt.Errorf("could not install the promise detector")
	}
	r.isPromise = isPromise
	return r, nil
}

// print collects text output. The completion value is reported separately, so
// print is for narration and intermediate values, mirroring a REPL.
func (r *JSRuntime) print(call goja.FunctionCall) goja.Value {
	parts := make([]string, 0, len(call.Arguments))
	for _, arg := range call.Arguments {
		parts = append(parts, arg.String())
	}
	r.output = append(r.output, strings.Join(parts, " "))
	return goja.Undefined()
}

// callTool is every cua.* method: one object argument in, one result object
// out. Driver failures throw as JavaScript errors so the model sees the
// driver's own message and can react, rather than getting a harness abort.
func (r *JSRuntime) callTool(name string, call goja.FunctionCall) goja.Value {
	args := map[string]any{}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) && !goja.IsNull(call.Arguments[0]) {
		exported := call.Arguments[0].Export()
		m, ok := exported.(map[string]any)
		if !ok {
			r.throwError("cua." + name + " takes a single object argument")
		}
		args = pruneNulls(m)
	}
	ctx := r.evalCtx
	if ctx == nil {
		ctx = context.Background()
	}
	// element_id is ours, not the driver's: resolve it to a fresh token now
	// (the wire inputs deny unknown fields, so it must never be forwarded).
	var resolved *resolvedElement
	if _, ok := args["element_id"]; ok {
		var err error
		resolved, err = r.resolveElementArg(ctx, name, args)
		if err != nil {
			r.throwError(err.Error())
		}
	}
	// wait_timeout_ms tunes only the launch wait below; strip it for the same
	// reason.
	waitTimeoutMs := 0
	if rawTimeout, ok := args["wait_timeout_ms"]; ok {
		delete(args, "wait_timeout_ms")
		if timeout, ok := toInt64(rawTimeout); ok && timeout > 0 {
			waitTimeoutMs = int(timeout)
		}
	}
	result, err := r.caller.Call(ctx, name, args, r.cancel)
	if err != nil {
		r.throwError("cua." + name + " failed: " + err.Error())
	}
	if resolved != nil && resolved.note != "" && !result.IsError {
		// Confirm what was actually touched: the id requested, the row it
		// landed on, and the role and label the tree displayed. Only for
		// successful acts — a failed call has nothing to confirm — and only
		// for the element_id path, which is the only one that resolved
		// anything.
		result.Content = append(result.Content, ContentPart{Type: "text", Text: resolved.note})
	}
	if name == "launch_app" {
		if note := r.waitLaunchWindow(ctx, args, result, waitTimeoutMs); note != "" {
			result.Content = append(result.Content, ContentPart{Type: "text", Text: note})
		}
	}
	return r.resultObject(result)
}

// throwError raises a JavaScript Error from native code. Panicking with a
// plain Go string would NOT become a catchable exception: the vm only
// converts goja.Values, and anything else propagates as a Go panic that Eval
// could mistake for an interrupt.
func (r *JSRuntime) throwError(message string) {
	ctor, ok := goja.AssertFunction(r.rt.Get("Error"))
	if !ok {
		panic(r.rt.ToValue(message))
	}
	errObj, err := ctor(goja.Undefined(), r.rt.ToValue(message))
	if err != nil {
		panic(r.rt.ToValue(message))
	}
	panic(errObj)
}

// pruneNulls drops null-valued keys. goja exports explicit undefined fields
// as nil, and the driver fails closed on mistyped arguments, so passing them
// through would turn an omitted option into a rejection.
func pruneNulls(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		if value == nil {
			continue
		}
		out[key] = value
	}
	return out
}

// resultObject renders a driver result for script code: text and image parts
// plus the typed structured view the next call's arguments come from.
// structuredContent is always an object, never null: the refusal path used
// to omit it, which threw TypeErrors in scripts accessing it, so an empty
// result carries {} and the shape never changes between paths.
func (r *JSRuntime) resultObject(result *ToolResult) goja.Value {
	content := make([]any, 0, len(result.Content))
	for _, part := range result.Content {
		switch part.Type {
		case "text":
			content = append(content, map[string]any{"type": "text", "text": part.Text})
		case "image":
			content = append(content, map[string]any{
				"type": "image", "data": base64.StdEncoding.EncodeToString(part.Data), "mimeType": part.MIMEType,
			})
		}
	}
	var structured any = map[string]any{}
	if len(result.StructuredContent) > 0 {
		var decoded any
		if json.Unmarshal(result.StructuredContent, &decoded) == nil && decoded != nil {
			structured = decoded
		}
	}
	return r.rt.ToValue(map[string]any{
		"content":           content,
		"structuredContent": structured,
		"isError":           result.IsError,
	})
}

// Eval runs code and converts the outcome to harness content parts. The
// returned error is nil on success; a *tools.ToolError carries a model-visible
// failure (bad code, driver refusal), anything else aborts the turn.
func (r *JSRuntime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	if strings.TrimSpace(code) == "" {
		return nil, tools.NewToolError("no code to run")
	}
	if len(code) > maxJSCodeBytes {
		return nil, tools.NewToolError("code is %d bytes, over the %d limit", len(code), maxJSCodeBytes)
	}
	if timeout <= 0 {
		timeout = jsCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.output = nil
	r.evalCtx = callCtx
	defer func() { r.evalCtx = nil }()

	// A runaway script must die on abort rather than pin the turn: watching
	// ctx and interrupting matches the driver's own cancel-every-50ms
	// contract. goja delivers the interrupt as an *InterruptedError panic,
	// which is uncatchable by script code; genuine JS throws arrive as the
	// error return instead. A stale interrupt flag would poison the next
	// evaluation, so it is cleared up front while no other eval can run.
	r.rt.ClearInterrupt()

	interrupted := false
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-callCtx.Done():
			r.rt.Interrupt("computer stopped")
		case <-done:
		}
	}()

	var completion goja.Value
	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				if _, ok := recovered.(*goja.InterruptedError); ok {
					interrupted = true
					r.rt.ClearInterrupt()
					return
				}
				// Anything else is a genuine Go bug, not a stop: re-panic
				// rather than misreporting it as an interrupted action.
				panic(recovered)
			}
		}()
		completion, runErr = r.rt.RunString(code)
	}()

	if interrupted || isInterrupt(runErr) {
		// An interrupted action may already have taken effect on screen, so
		// its outcome is unknown: never let the model assume it did not
		// happen. This is the same wording Stop uses everywhere else.
		return nil, tools.NewToolError("computer stopped; action completion is unknown. Inspect fresh state before retrying.")
	}
	if runErr != nil {
		return nil, tools.NewToolError("JavaScript error: %s", jsErrorText(runErr))
	}
	if isPromise, err := r.isPromise(goja.Undefined(), completion); err == nil && isPromise.ToBoolean() {
		return nil, tools.NewToolError("async code is not supported: call cua methods directly, they already wait for the result")
	}
	parts, err := r.renderCompletion(completion)
	if err != nil {
		return nil, err
	}
	return parts, nil
}

// isInterrupt reports whether a RunString error is an interrupt that arrived
// as an error return rather than a panic. Only the exact type counts: script
// throws never produce it, so there is no message sniffing to misfire.
func isInterrupt(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*goja.InterruptedError)
	return ok
}

// jsErrorText renders a script failure with its message, truncated so a long
// stack cannot flood the context.
func jsErrorText(err error) string {
	text := err.Error()
	if exception, ok := err.(*goja.Exception); ok {
		if stack := exception.Stack(); len(stack) > 0 {
			frame := stack[0]
			text += fmt.Sprintf("\n  at %s (%s)", frame.FuncName(), frame.Position())
		}
	}
	if len(text) > 2000 {
		text = text[:2000] + "… [truncated]"
	}
	return text
}

// renderCompletion converts print output plus the completion value to harness
// parts. Image-shaped objects anywhere in the completion become native image
// parts; everything else becomes text.
func (r *JSRuntime) renderCompletion(completion goja.Value) ([]map[string]any, error) {
	var texts []string
	texts = append(texts, r.output...)
	images, notes, err := collectImages(completion)
	if err != nil {
		return nil, err
	}
	texts = append(texts, notes...)
	if completion != nil && !goja.IsUndefined(completion) {
		if rendered := stringifyCompletion(completion); rendered != "" {
			texts = append(texts, rendered)
		}
	}
	joined := strings.Join(texts, "\n")
	if strings.TrimSpace(joined) == "" && len(images) == 0 {
		joined = "(no output)"
	}
	if len(joined) > maxResultText {
		joined = joined[:maxResultText] + "\n… [truncated]"
	}
	out := []map[string]any{{"type": "text", "text": joined}}
	return append(out, images...), nil
}

// stringifyCompletion renders the completion value as JSON, which is how the
// model reads structured observations back.
func stringifyCompletion(completion goja.Value) string {
	exported := completion.Export()
	if exported == nil {
		return ""
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		return ""
	}
	text := string(raw)
	if text == "null" || text == "{}" || text == "[]" {
		return ""
	}
	return text
}

// collectImages walks the completion value for image-shaped objects and
// converts them to harness parts. Data must be valid base64: a hand-built
// image object with garbage data fails loudly rather than reaching the
// provider as a corrupt block.
func collectImages(completion goja.Value) ([]map[string]any, []string, error) {
	if completion == nil || goja.IsUndefined(completion) {
		return nil, nil, nil
	}
	exported := completion.Export()
	found := findImageShapes(exported)
	var out []map[string]any
	var notes []string
	for i, shape := range found {
		if i >= maxJSImages {
			notes = append(notes, fmt.Sprintf("[%d more image(s) omitted]", len(found)-maxJSImages))
			break
		}
		if _, err := base64.StdEncoding.DecodeString(shape.data); err != nil {
			return nil, nil, tools.NewToolError("image data is not valid base64")
		}
		mimeType := shape.mimeType
		if mimeType == "" {
			mimeType = "image/png"
		}
		out = append(out, map[string]any{"type": "image", "data": shape.data, "mimeType": mimeType})
	}
	return out, notes, nil
}

type imageShape struct {
	data     string
	mimeType string
}

// findImageShapes collects every {type:'image', data} object in the value,
// descending into arrays and objects. Only exact type matches count: a text
// field that happens to mention images is left alone.
func findImageShapes(value any) []imageShape {
	var out []imageShape
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if kind, _ := v["type"].(string); kind == "image" {
				if data, _ := v["data"].(string); data != "" {
					mimeType, _ := v["mimeType"].(string)
					out = append(out, imageShape{data: data, mimeType: mimeType})
					return
				}
			}
			for _, item := range v {
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(value)
	return out
}
