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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MoeForge/moejs"
	"github.com/MoeForge/moejs/eventloop"
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

// OnlyArgs refuses arguments a method does not take. Silently ignoring an
// unknown key hides typos ({projectId: ...}); naming the accepted keys turns
// the mistake into a one-shot fix.
func OnlyArgs(args map[string]any, allowed ...string) error {
	for key := range args {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known {
			return Errorf("unknown argument %q; this method takes: %s", key, strings.Join(allowed, ", "))
		}
	}
	return nil
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
	// AsyncMessage is deprecated and unused. Async code and Promises are supported.
	AsyncMessage string
	// MemoryLimit caps JavaScript heap allocation in bytes. 0 means unbounded.
	MemoryLimit int64
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
	return o
}

// Runtime is one persistent JavaScript interpreter with a preinstalled root
// object. Runtimes are guarded by mu for serialized evaluation.
type Runtime struct {
	mu      sync.Mutex
	rt      *moejs.Runtime
	loop    *eventloop.Loop
	opts    Options
	method  map[string]Method
	output  []string
	evalCtx context.Context
	cancel  func() bool
}

// New builds a runtime and installs print plus the root object from methods.
func New(opts Options, methods []Method) (*Runtime, error) {
	opts = opts.withDefaults()
	moeOpts := moejs.Options{}
	if opts.MemoryLimit > 0 {
		moeOpts.MemoryLimit = opts.MemoryLimit
	}
	rt := moejs.NewRuntime(moeOpts)
	loop, err := eventloop.New(rt, eventloop.Options{})
	if err != nil {
		return nil, err
	}
	r := &Runtime{
		rt:     rt,
		loop:   loop,
		opts:   opts,
		method: make(map[string]Method, len(methods)),
	}

	if err := r.rt.SetGlobal("print", moejs.NativeFunc(r.print)); err != nil {
		return nil, err
	}

	rootMap := make(map[string]any)
	for _, method := range methods {
		if strings.TrimSpace(method.Name) == "" || method.Call == nil {
			continue
		}
		name := method.Name
		r.method[name] = method

		parts := strings.Split(name, ".")
		cur := rootMap
		for _, part := range parts[:len(parts)-1] {
			if part == "" {
				return nil, fmt.Errorf("empty path segment in %q", name)
			}
			next, ok := cur[part].(map[string]any)
			if !ok {
				next = make(map[string]any)
				cur[part] = next
			}
			cur = next
		}
		leaf := parts[len(parts)-1]
		cur[leaf] = moejs.NativeFunc(func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
			return r.callMethod(name, realm, args)
		})
	}

	if err := r.rt.SetGlobal(opts.Root, rootMap); err != nil {
		return nil, err
	}

	if opts.Root != "console" {
		_ = r.rt.SetGlobal("console", map[string]any{
			"log": moejs.NativeFunc(r.print),
		})
	}

	if err := r.guardUnknownMembers(opts.Root); err != nil {
		return nil, err
	}

	return r, nil
}

// SetCancel installs the hook polled while a method call is in flight.
func (r *Runtime) SetCancel(fn func() bool) {
	r.cancel = fn
}

// print collects text output. The completion value is reported separately, so
// print is for narration and intermediate values, mirroring a REPL.
func (r *Runtime) print(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		val, err := r.rt.ToGo(arg)
		if err != nil {
			parts = append(parts, fmt.Sprint(arg))
			continue
		}
		if s, ok := val.(string); ok {
			parts = append(parts, s)
		} else if val == nil {
			parts = append(parts, "null")
		} else {
			raw, err := json.Marshal(val)
			if err == nil {
				parts = append(parts, string(raw))
			} else {
				parts = append(parts, fmt.Sprint(val))
			}
		}
	}
	r.output = append(r.output, strings.Join(parts, " "))
	return moejs.Undefined(), nil
}

// callMethod is every root.* method: one object argument in, one result
// object out. Method failures throw as JavaScript errors so the model sees the
// caller's own message and can react, rather than getting a harness abort.
func (r *Runtime) callMethod(name string, realm *moejs.Realm, args []moejs.Value) (moejs.Value, error) {
	argsMap := map[string]any{}
	if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
		exported, err := r.rt.ToGo(args[0])
		if err != nil {
			return moejs.Undefined(), fmt.Errorf("%s.%s: %w", r.opts.Root, name, err)
		}
		m, ok := exported.(map[string]any)
		if !ok {
			return moejs.Undefined(), fmt.Errorf("%s.%s takes a single object argument", r.opts.Root, name)
		}
		argsMap = pruneNulls(m)
	}
	ctx := r.evalCtx
	if ctx == nil {
		ctx = context.Background()
	}
	method, ok := r.method[name]
	if !ok {
		return moejs.Undefined(), fmt.Errorf("%s.%s is not installed", r.opts.Root, name)
	}
	result, err := method.Call(ctx, argsMap)
	if err != nil {
		return moejs.Undefined(), err
	}
	return r.resultObject(result)
}

// pruneNulls drops null-valued keys.
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
func (r *Runtime) resultObject(result *Result) (moejs.Value, error) {
	if result == nil {
		result = &Result{}
	}
	if r.opts.PlainResults && !result.IsError && len(result.Content) == 0 && len(result.StructuredContent) > 0 {
		var decoded any
		if json.Unmarshal(result.StructuredContent, &decoded) == nil && decoded != nil {
			return r.rt.FromGo(decoded)
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
	return r.rt.FromGo(map[string]any{
		"content":           content,
		"structuredContent": structured,
		"isError":           result.IsError,
	})
}

// wrapTopLevelAwait compiles code directly as a script, or wraps top-level
// await in an async IIFE if top-level await is encountered.
func wrapTopLevelAwait(code string) (*moejs.Script, error) {
	s, err := moejs.CompileScript("eval.js", code)
	if err == nil {
		return s, nil
	}
	if !strings.Contains(code, "await") {
		return nil, err
	}
	trimmed := strings.TrimSpace(code)
	if sExpr, errExpr := moejs.CompileScript("eval.js", "(async () => (\n"+trimmed+"\n))()"); errExpr == nil {
		return sExpr, nil
	}
	statements := strings.Split(trimmed, "\n")
	if len(statements) > 0 {
		lastIdx := len(statements) - 1
		for lastIdx >= 0 && strings.TrimSpace(statements[lastIdx]) == "" {
			lastIdx--
		}
		if lastIdx >= 0 {
			last := strings.TrimSpace(statements[lastIdx])
			lastWithoutSemi := strings.TrimSuffix(last, ";")
			if !strings.HasPrefix(last, "return ") && !strings.HasPrefix(last, "const ") && !strings.HasPrefix(last, "let ") && !strings.HasPrefix(last, "var ") {
				statementsCopy := make([]string, len(statements))
				copy(statementsCopy, statements)
				statementsCopy[lastIdx] = "return (" + lastWithoutSemi + ");"
				wrapped := "(async () => {\n" + strings.Join(statementsCopy, "\n") + "\n})()"
				if sRet, errRet := moejs.CompileScript("eval.js", wrapped); errRet == nil {
					return sRet, nil
				}
			}
		}
	}
	if sBlock, errBlock := moejs.CompileScript("eval.js", "(async () => {\n"+trimmed+"\n})()"); errBlock == nil {
		return sBlock, nil
	}
	return nil, err
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

	s, compileErr := wrapTopLevelAwait(code)
	if compileErr != nil {
		return nil, Errorf("JavaScript error: %s", jsErrorText(compileErr))
	}

	r.rt.ClearInterrupt()

	stop := context.AfterFunc(callCtx, func() {
		r.loop.Interrupt(context.Cause(callCtx))
	})
	defer stop()

	var completion moejs.Value = moejs.Undefined()
	var completionErr error
	var panicked any

	runErr := func() (rErr error) {
		defer func() {
			if p := recover(); p != nil {
				panicked = p
			}
		}()
		return r.loop.Run(func(rt *moejs.Runtime) error {
			val, err := rt.RunScript(s)
			if err != nil {
				return err
			}
			if _, _, ok := moejs.PromiseResult(val); ok {
				holdDone := r.loop.Hold()
				then, err := rt.Get(val, "then")
				if err != nil {
					holdDone(nil)
					return err
				}
				onResolve := rt.Function("onResolve", 1, func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
					completion = moejs.Arg(args, 0)
					holdDone(nil)
					return moejs.Undefined(), nil
				})
				onReject := rt.Function("onReject", 1, func(realm *moejs.Realm, this moejs.Value, args []moejs.Value) (moejs.Value, error) {
					arg := moejs.Arg(args, 0)
					if exc, ok := r.rt.ToGo(arg); ok == nil && exc != nil {
						completionErr = fmt.Errorf("%v", exc)
					} else {
						completionErr = errors.New("promise rejected")
					}
					holdDone(nil)
					return moejs.Undefined(), nil
				})
				_, err = rt.CallFunction(then, val, onResolve, onReject)
				return err
			}
			completion = val
			return nil
		})
	}()

	if panicked != nil {
		if isInterruptErr(panicked) {
			r.rt.ClearInterrupt()
			return nil, Errorf("%s", r.opts.InterruptMessage)
		}
		panic(panicked)
	}

	if isInterrupt(runErr) {
		r.rt.ClearInterrupt()
		return nil, Errorf("%s", r.opts.InterruptMessage)
	}
	if runErr != nil {
		return nil, Errorf("JavaScript error: %s", jsErrorText(runErr))
	}
	if completionErr != nil {
		return nil, Errorf("JavaScript error: %s", jsErrorText(completionErr))
	}
	return r.renderCompletion(completion)
}

func isInterrupt(err error) bool {
	if err == nil {
		return false
	}
	var ie *moejs.InterruptedError
	return errors.As(err, &ie)
}

func isInterruptErr(v any) bool {
	if v == nil {
		return false
	}
	if err, ok := v.(error); ok {
		var ie *moejs.InterruptedError
		return errors.As(err, &ie)
	}
	return false
}

func jsErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if len(text) > 2000 {
		text = text[:2000] + "… [truncated]"
	}
	return text
}

// renderCompletion converts print output plus the completion value to harness
// parts. Image-shaped objects anywhere in the completion become native image
// parts; everything else becomes text.
func (r *Runtime) renderCompletion(completion moejs.Value) ([]map[string]any, error) {
	var texts []string
	texts = append(texts, r.output...)
	images, notes, err := r.collectImages(completion)
	if err != nil {
		return nil, err
	}
	texts = append(texts, notes...)
	if !completion.IsUndefined() {
		if rendered := r.stringifyCompletion(completion); rendered != "" {
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
func (r *Runtime) stringifyCompletion(completion moejs.Value) string {
	exported, err := r.rt.ToGo(completion)
	if err != nil || exported == nil {
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
func (r *Runtime) collectImages(completion moejs.Value) ([]map[string]any, []string, error) {
	if completion.IsUndefined() {
		return nil, nil, nil
	}
	exported, err := r.rt.ToGo(completion)
	if err != nil || exported == nil {
		return nil, nil, nil
	}
	found := findImageShapes(exported)
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
