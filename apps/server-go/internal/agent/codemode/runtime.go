package codemode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Caller executes tool calls and exposes the tool registry.
type Caller interface {
	Execute(ctx context.Context, call tools.ToolCall) (tools.ToolResult, error)
	Registry() *tools.Registry
}

// CallTrace records one tool execution inside a script.
type CallTrace struct {
	Tool     string
	Summary  string
	Duration time.Duration
	IsError  bool
	ErrMsg   string
}

// Runtime is one persistent JavaScript REPL session for codemode.
type Runtime struct {
	mu     sync.Mutex
	kernel *jskernel.Runtime
	caller Caller
	store  map[string]any
	traces []CallTrace
}

// NewRuntime builds and initializes a persistent codemode interpreter.
func NewRuntime(caller Caller) (*Runtime, error) {
	r := &Runtime{
		caller: caller,
		store:  make(map[string]any),
	}

	methods := []jskernel.Method{
		{
			Name: "call",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.dispatchCall(ctx, args)
			},
		},
		{
			Name: "searchTools",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.searchTools(ctx, args)
			},
		},
		{
			Name: "describeTool",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.describeTool(ctx, args)
			},
		},
		{
			Name: "store",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.setStore(ctx, args)
			},
		},
		{
			Name: "load",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				return r.getStore(ctx, args)
			},
		},
		{
			Name: "preview",
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				raw, _ := json.Marshal(args["data"])
				return &jskernel.Result{StructuredContent: raw}, nil
			},
		},
	}

	opts := jskernel.Options{
		Root:         "__harness",
		PlainResults: true,
		Timeout:      10 * time.Minute,
	}

	kernel, err := jskernel.New(opts, methods)
	if err != nil {
		return nil, err
	}
	r.kernel = kernel

	if err := r.kernel.RunInitScript(bootstrapScript); err != nil {
		return nil, fmt.Errorf("codemode bootstrap failed: %w", err)
	}

	return r, nil
}

// SetCaller updates the caller used for tool execution.
func (r *Runtime) SetCaller(caller Caller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caller = caller
}

// Reset clears session-scoped store variables and rebuilds the kernel.
func (r *Runtime) Reset() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store = make(map[string]any)
	r.traces = nil

	fresh, err := NewRuntime(r.caller)
	if err != nil {
		return err
	}
	r.kernel = fresh.kernel
	return nil
}

// Eval executes JavaScript code against the persistent session kernel.
func (r *Runtime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	r.mu.Lock()
	r.traces = nil
	r.mu.Unlock()

	res, evalErr := r.kernel.Eval(ctx, code, timeout)

	r.mu.Lock()
	traces := make([]CallTrace, len(r.traces))
	copy(traces, r.traces)
	r.mu.Unlock()

	traceText := formatTraces(traces)

	if evalErr != nil {
		msg := evalErr.Error()
		if traceText != "" {
			msg += "\n\n" + traceText
		}
		return tools.Envelope{Content: msg, IsError: true}, nil
	}

	outputText := renderOutputText(res)
	if outputText == "" && traceText == "" {
		outputText = "Done (no output)"
	} else if traceText != "" {
		if outputText != "" {
			outputText += "\n\n" + traceText
		} else {
			outputText = traceText
		}
	}

	return tools.Envelope{Content: outputText, IsError: false}, nil
}

func (r *Runtime) recordTrace(trace CallTrace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traces = append(r.traces, trace)
}

func (r *Runtime) dispatchCall(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	toolName, _ := args["tool"].(string)
	if toolName == "" {
		return nil, errors.New("tool name is required")
	}

	// Normalize aliases
	switch toolName {
	case "edit_file":
		toolName = "editFile"
	case "read_chat":
		toolName = "readChat"
	case "web_fetch", "fetch":
		toolName = "webFetch"
	case "web_search":
		toolName = "webSearch"
	case "bash_job":
		toolName = "bashJob"
	case "batch_write":
		toolName = "batchWrite"
	}

	input, _ := args["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}

	summary := formatSummary(toolName, input)

	r.mu.Lock()
	caller := r.caller
	r.mu.Unlock()

	if caller == nil {
		return nil, errors.New("tool caller not available")
	}

	rawArgs, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal tool input: %w", err)
	}

	id := generateCallID()
	start := time.Now()
	res, execErr := caller.Execute(ctx, tools.ToolCall{
		ID:        id,
		Name:      toolName,
		Arguments: rawArgs,
	})
	dur := time.Since(start)

	if execErr != nil {
		r.recordTrace(CallTrace{
			Tool:     toolName,
			Summary:  summary,
			Duration: dur,
			IsError:  true,
			ErrMsg:   execErr.Error(),
		})
		return nil, execErr
	}

	if res.IsError {
		errMsg := formatContentString(res.Content)
		r.recordTrace(CallTrace{
			Tool:     toolName,
			Summary:  summary,
			Duration: dur,
			IsError:  true,
			ErrMsg:   errMsg,
		})
		return nil, errors.New(errMsg)
	}

	r.recordTrace(CallTrace{
		Tool:     toolName,
		Summary:  summary,
		Duration: dur,
		IsError:  false,
	})

	unwrapped := unwrapContent(res.Content)
	raw, marshalErr := json.Marshal(unwrapped)
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal tool output: %w", marshalErr)
	}

	return &jskernel.Result{StructuredContent: raw}, nil
}

func (r *Runtime) searchTools(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	query := strings.ToLower(fmt.Sprint(args["query"]))

	r.mu.Lock()
	caller := r.caller
	r.mu.Unlock()

	var matches []map[string]any
	if caller != nil && caller.Registry() != nil {
		for _, t := range caller.Registry().Tools() {
			name := t.Name()
			desc := t.Description()
			if query == "" || strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(desc), query) {
				matches = append(matches, map[string]any{
					"name":        name,
					"description": desc,
				})
			}
		}
	}

	raw, _ := json.Marshal(matches)
	return &jskernel.Result{StructuredContent: raw}, nil
}

func (r *Runtime) describeTool(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	name := fmt.Sprint(args["name"])

	r.mu.Lock()
	caller := r.caller
	r.mu.Unlock()

	if caller == nil || caller.Registry() == nil {
		return nil, fmt.Errorf("tool registry unavailable")
	}

	t, err := caller.Registry().Get(name)
	if err != nil {
		return nil, err
	}

	schema, err := tools.SchemaMap(t)
	if err != nil {
		return nil, err
	}

	raw, _ := json.Marshal(map[string]any{
		"name":        t.Name(),
		"description": t.Description(),
		"parameters":  schema,
	})
	return &jskernel.Result{StructuredContent: raw}, nil
}

func (r *Runtime) setStore(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	key := fmt.Sprint(args["key"])
	val := args["value"]

	r.mu.Lock()
	r.store[key] = val
	r.mu.Unlock()

	raw, _ := json.Marshal(true)
	return &jskernel.Result{StructuredContent: raw}, nil
}

func (r *Runtime) getStore(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	key := fmt.Sprint(args["key"])

	r.mu.Lock()
	val := r.store[key]
	r.mu.Unlock()

	raw, _ := json.Marshal(val)
	return &jskernel.Result{StructuredContent: raw}, nil
}

func unwrapContent(content any) any {
	if parts, ok := content.([]map[string]any); ok && len(parts) == 1 {
		if parts[0]["type"] == "text" {
			return parts[0]["text"]
		}
	}
	return content
}

func formatContentString(content any) string {
	if str, ok := content.(string); ok {
		return str
	}
	if parts, ok := content.([]map[string]any); ok && len(parts) > 0 {
		var sb strings.Builder
		for i, part := range parts {
			if i > 0 {
				sb.WriteString("\n")
			}
			if text, ok := part["text"].(string); ok {
				sb.WriteString(text)
			}
		}
		return sb.String()
	}
	raw, err := json.Marshal(content)
	if err == nil {
		return string(raw)
	}
	return fmt.Sprint(content)
}

func formatSummary(tool string, input map[string]any) string {
	switch tool {
	case "read_file":
		return fmt.Sprintf("file(%v)", input["path"])
	case "write_file":
		return fmt.Sprintf("write(%v)", input["path"])
	case "editFile":
		return fmt.Sprintf("edit(%v)", input["path"])
	case "bash":
		cmd := fmt.Sprint(input["command"])
		if len(cmd) > 50 {
			cmd = cmd[:47] + "..."
		}
		return fmt.Sprintf("$ %s", cmd)
	case "glob":
		return fmt.Sprintf("glob(%v)", input["pattern"])
	case "grep":
		return fmt.Sprintf("grep(%v)", input["pattern"])
	case "list_dir":
		return fmt.Sprintf("list(%v)", input["path"])
	case "webFetch":
		return fmt.Sprintf("fetch(%v)", input["url"])
	case "webSearch":
		return fmt.Sprintf("webSearch(%v)", input["query"])
	case "todo":
		return fmt.Sprintf("todo(%v)", input["op"])
	case "visualize":
		return "visualize(...)"
	case "readChat":
		return fmt.Sprintf("readChat(%v)", input["ref"])
	default:
		return tool
	}
}

func formatTraces(traces []CallTrace) string {
	if len(traces) == 0 {
		return ""
	}
	lines := make([]string, 0, len(traces)+2)
	lines = append(lines, fmt.Sprintf("--- Calls (%d) ---", len(traces)))
	for _, tr := range traces {
		durStr := fmt.Sprintf("%dms", tr.Duration.Milliseconds())
		if tr.IsError {
			errPreview := tr.ErrMsg
			if len(errPreview) > 60 {
				errPreview = errPreview[:57] + "..."
			}
			lines = append(lines, fmt.Sprintf("✗ %s (%s) — %s", tr.Summary, durStr, errPreview))
		} else {
			lines = append(lines, fmt.Sprintf("✓ %s (%s)", tr.Summary, durStr))
		}
	}
	return strings.Join(lines, "\n")
}

func renderOutputText(res any) string {
	if res == nil {
		return ""
	}
	parts, ok := res.([]map[string]any)
	if !ok || len(parts) == 0 {
		return fmt.Sprint(res)
	}
	var texts []string
	for _, part := range parts {
		if text, ok := part["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func generateCallID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}
