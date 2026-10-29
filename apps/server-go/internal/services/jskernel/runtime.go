// Package jskernel is the shared JavaScript interpreter behind harness tools
// that expose a method API instead of one schema per operation.
//
// One Runtime is one persistent interpreter: variables and bindings survive
// between evaluations until the runtime is discarded. Methods are installed
// under a single root object and may use dotted names, so an API can group
// methods (console.sessions.read) without one JS object per namespace.
//
// Callers own the method implementations and the limits they pass. The runtime
// owns evaluation, print collection, completion rendering, image extraction,
// timeouts and interrupts, so every caller gets identical failure semantics.
package jskernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// Error is a model-visible runtime failure: bad code, an interrupted
// evaluation, or a refused method call. The runtime stays independent of the
// tool framework; callers that speak its error type convert this at the
// boundary.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// Errorf builds a model-visible runtime failure.
func Errorf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// Part is one content part a method returns to script code.
type Part struct {
	Type     string // "text" or "image"
	Text     string
	Data     []byte
	MIMEType string
}

// Result is what a method returns to script code. StructuredContent is the
// typed view scripts index directly; it is always rendered as an object, never
// null, so a method's result shape does not change between success and failure.
type Result struct {
	Content           []Part
	StructuredContent json.RawMessage
	IsError           bool
}

// Method is one callable installed under the runtime's root object. Name may
// contain dots ("sessions.read") to build nested objects.
type Method struct {
	Name string
	Call func(ctx context.Context, args map[string]any) (*Result, error)
}

// Options configures one runtime's limits and wording. Zero values select the
// defaults below, so a caller only sets what it needs to change.
type Options struct {
	// Root is the object scripts call, such as "cua" or "console".
	Root string
	// Timeout is the default budget for one evaluation.
	Timeout time.Duration
	// MaxCodeBytes caps the source text accepted in one call.
	MaxCodeBytes int
	// MaxResultText caps the text pulled out of one evaluation.
	MaxResultText int
	// MaxImages caps inline images carried by one completion.
	MaxImages int
	// InterruptMessage is reported when an evaluation is interrupted. Callers
	// whose actions may already have taken effect should say so here.
	InterruptMessage string
	// PlainResults makes root.* methods return their structured data directly
	// instead of the {content, structuredContent, isError} wrapper. A method
	// that returned only text or images still gets the wrapper, so nothing is
	// lost. Off by default: computer use relies on the wrapper for images.
	PlainResults bool
	// AsyncMessage is reported when a script returns a Promise. No microtask
	// pump runs between evaluations, so a promise would silently never settle.
	AsyncMessage string
}

// Defaults applied by New.
const (
	DefaultTimeout       = 5 * time.Minute
	DefaultMaxCodeBytes  = 200_000
	DefaultMaxResultText = 100_000
	DefaultMaxImages     = 4
)

func (o Options) withDefaults() Options {
	if o.Root == "" {
		o.Root = "kernel"
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxCodeBytes <= 0 {
		o.MaxCodeBytes = DefaultMaxCodeBytes
	}
	if o.MaxResultText <= 0 {
		o.MaxResultText = DefaultMaxResultText
	}
	if o.MaxImages <= 0 {
		o.MaxImages = DefaultMaxImages
	}
	if o.InterruptMessage == "" {
		o.InterruptMessage = "execution stopped; completion is unknown. Inspect fresh state before retrying."
	}
	if o.AsyncMessage == "" {
		o.AsyncMessage = "async code is not supported: call methods directly, they already wait for the result"
	}
	return o
}

// Runtime is one persistent JavaScript interpreter with a preinstalled root
// object. goja runtimes are not safe for concurrent use, so every entry takes
// mu — including the interrupt path, which goja documents as safe to call from
// another goroutine.
type Runtime struct {
	mu     sync.Mutex
	rt     *goja.Runtime
	opts   Options
	method map[string]Method
	output []string
	// evalCtx is the context of the evaluation currently running on the Eval
	// goroutine; methods read it without locking because only that goroutine
	// touches it.
	evalCtx context.Context
	// cancel is polled while a method call is in flight, so an external stop
	// interrupts admitted work. nil means no external stop source.
	cancel    func() bool
	isPromise func(goja.Value, ...goja.Value) (goja.Value, error)
}

// New builds a runtime and installs print plus the root object from methods.
func New(opts Options, methods []Method) (*Runtime, error) {
	opts = opts.withDefaults()
	r := &Runtime{rt: goja.New(), opts: opts, method: make(map[string]Method, len(methods))}
	r.rt.Set("print", r.print)

	root := r.rt.NewObject()
	for _, method := range methods {
		if strings.TrimSpace(method.Name) == "" || method.Call == nil {
			continue
		}
		name := method.Name
		r.method[name] = method
		if err := installMethod(r.rt, root, name, func(call goja.FunctionCall) goja.Value {
			return r.callMethod(name, call)
		}); err != nil {
			return nil, fmt.Errorf("install %s.%s: %w", opts.Root, name, err)
		}
	}
	if err := r.rt.Set(opts.Root, root); err != nil {
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

// installMethod walks a dotted name, creating intermediate objects, and sets
// the leaf. Flat names keep working unchanged.
func installMethod(rt *goja.Runtime, root *goja.Object, name string, fn func(goja.FunctionCall) goja.Value) error {
	parts := strings.Split(name, ".")
	obj := root
	for _, part := range parts[:len(parts)-1] {
		if part == "" {
			return fmt.Errorf("empty path segment")
		}
		existing := obj.Get(part)
		if existing == nil || goja.IsUndefined(existing) || goja.IsNull(existing) {
			child := rt.NewObject()
			if err := obj.Set(part, child); err != nil {
				return err
			}
			obj = child
			continue
		}
		child, ok := existing.(*goja.Object)
		if !ok {
			return fmt.Errorf("%q is not an object", part)
		}
		obj = child
	}
	return obj.Set(parts[len(parts)-1], fn)
}

// SetCancel installs the hook polled while a method call is in flight.
func (r *Runtime) SetCancel(fn func() bool) {
	r.cancel = fn
}

// print collects text output. The completion value is reported separately, so
// print is for narration and intermediate values, mirroring a REPL.
func (r *Runtime) print(call goja.FunctionCall) goja.Value {
	parts := make([]string, 0, len(call.Arguments))
	for _, arg := range call.Arguments {
		parts = append(parts, arg.String())
	}
	r.output = append(r.output, strings.Join(parts, " "))
	return goja.Undefined()
}

// callMethod is every root.* method: one object argument in, one result
// object out. Method failures throw as JavaScript errors so the model sees the
// caller's own message and can react, rather than getting a harness abort.
func (r *Runtime) callMethod(name string, call goja.FunctionCall) goja.Value {
	args := map[string]any{}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) && !goja.IsNull(call.Arguments[0]) {
		m, ok := call.Arguments[0].Export().(map[string]any)
		if !ok {
			r.throwError(r.opts.Root + "." + name + " takes a single object argument")
		}
		args = pruneNulls(m)
	}
	ctx := r.evalCtx
	if ctx == nil {
		ctx = context.Background()
	}
	method, ok := r.method[name]
	if !ok {
		r.throwError(r.opts.Root + "." + name + " is not installed")
	}
	result, err := method.Call(ctx, args)
	if err != nil {
		r.throwError(err.Error())
	}
	return r.resultObject(result)
}

// throwError raises a JavaScript Error from native code. Panicking with a plain
// Go string would NOT become a catchable exception: the vm only converts
// goja.Values, and anything else propagates as a Go panic that Eval could
// mistake for an interrupt.
func (r *Runtime) throwError(message string) {
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

// pruneNulls drops null-valued keys. goja exports explicit undefined fields as
// nil, and callers that fail closed on unknown arguments would otherwise turn
// an omitted option into a rejection.
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

// resultObject renders a method result for script code: text and image parts
// plus the typed structured view the next call's arguments come from.
func (r *Runtime) resultObject(result *Result) goja.Value {
	if result == nil {
		result = &Result{}
	}
	if r.opts.PlainResults && !result.IsError && len(result.Content) == 0 && len(result.StructuredContent) > 0 {
		var decoded any
		if json.Unmarshal(result.StructuredContent, &decoded) == nil && decoded != nil {
			return r.rt.ToValue(decoded)
		}
	}
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

// evalOutcome carries one evaluation's result across the worker boundary. The
// worker always sends exactly one, so Eval never leaks a goroutine and never
// confuses one evaluation's outcome with another's.
type evalOutcome struct {
	completion goja.Value
	runErr     error
	panicked   any
}

// Eval runs code and converts the outcome to harness content parts. The
// returned error is nil on success; a *Error carries a model-visible failure
// (bad code, method refusal), anything else aborts the turn.
func (r *Runtime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	if strings.TrimSpace(code) == "" {
		return nil, Errorf("no code to run")
	}
	if len(code) > r.opts.MaxCodeBytes {
		return nil, Errorf("code is %d bytes, over the %d limit", len(code), r.opts.MaxCodeBytes)
	}
	if ctx.Err() != nil {
		// Already cancelled before starting: nothing ran, so the outcome is
		// known (not unknown) — the action was refused, not interrupted.
		return nil, Errorf("execution cancelled before it started")
	}
	if timeout <= 0 {
		timeout = r.opts.Timeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.output = nil
	r.evalCtx = callCtx
	defer func() { r.evalCtx = nil }()

	// The script runs on a worker so an abort can interrupt it without any
	// cross-evaluation signalling: the outcome travels back on a per-eval
	// channel, so a late event from one evaluation can never be mistaken for
	// another's.
	r.rt.ClearInterrupt()
	resCh := make(chan evalOutcome, 1)
	go func() {
		var out evalOutcome
		defer func() {
			if p := recover(); p != nil {
				out.panicked = p
			}
			resCh <- out
		}()
		out.completion, out.runErr = r.rt.RunString(code)
	}()

	var out evalOutcome
	select {
	case out = <-resCh:
		// Finished before any abort.
	case <-callCtx.Done():
		// Abort while running: interrupt, then drain. The worker always sends
		// exactly one outcome, so this cannot hang.
		r.rt.Interrupt("execution stopped")
		out = <-resCh
	}

	if out.panicked != nil {
		if _, ok := out.panicked.(*goja.InterruptedError); ok {
			r.rt.ClearInterrupt()
			return nil, Errorf("%s", r.opts.InterruptMessage)
		}
		// Anything else is a genuine Go bug, not a stop: re-panic rather than
		// misreporting it as an interrupted action.
		panic(out.panicked)
	}
	completion, runErr := out.completion, out.runErr
	if isInterrupt(runErr) {
		r.rt.ClearInterrupt()
		return nil, Errorf("%s", r.opts.InterruptMessage)
	}
	if runErr != nil {
		return nil, Errorf("JavaScript error: %s", jsErrorText(runErr))
	}
	if isPromise, err := r.isPromise(goja.Undefined(), completion); err == nil && isPromise.ToBoolean() {
		return nil, Errorf("%s", r.opts.AsyncMessage)
	}
	return r.renderCompletion(completion)
}

// isInterrupt reports whether a RunString error is an interrupt that arrived as
// an error return rather than a panic. Only the exact type counts: script
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
func (r *Runtime) renderCompletion(completion goja.Value) ([]map[string]any, error) {
	var texts []string
	texts = append(texts, r.output...)
	images, notes, err := r.collectImages(completion)
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
	if len(joined) > r.opts.MaxResultText {
		cut := len(joined) - r.opts.MaxResultText
		joined = joined[:r.opts.MaxResultText] + fmt.Sprintf("\n… [truncated: %d more characters; request less, for example with a limit argument, or print only the fields you need]", cut)
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
// converts them to harness parts. Data must be valid base64: a hand-built image
// object with garbage data fails loudly rather than reaching the provider as a
// corrupt block.
func (r *Runtime) collectImages(completion goja.Value) ([]map[string]any, []string, error) {
	if completion == nil || goja.IsUndefined(completion) {
		return nil, nil, nil
	}
	found := findImageShapes(completion.Export())
	var out []map[string]any
	var notes []string
	for i, shape := range found {
		if i >= r.opts.MaxImages {
			notes = append(notes, fmt.Sprintf("[%d more image(s) omitted]", len(found)-r.opts.MaxImages))
			break
		}
		if _, err := base64.StdEncoding.DecodeString(shape.data); err != nil {
			return nil, nil, Errorf("image data is not valid base64")
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
