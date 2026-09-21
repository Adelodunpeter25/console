// Tool framework: struct-tag-driven schemas via invopop/jsonschema, with
// validation for free from the unmarshal step. Ports the shape of
// apps/server/agent/src/types/tool.ts (ToolCall/ToolResult/AgentTool).
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

// ToolTier is the capability class used for permission approval.
type ToolTier string

const (
	TierRead  ToolTier = "read"
	TierWrite ToolTier = "write"
	TierExec  ToolTier = "exec"
)

// ToolCall is the model's request to run a tool.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResult is the outcome sent back to the model.
type ToolResult struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName,omitempty"`
	Content    any    `json:"content"`
	IsError    bool   `json:"isError,omitempty"`
}

// ToolError signals a tool failure back to the harness (result carries it
// to the model instead of throwing).
type ToolError struct {
	Msg string
}
func (e *ToolError) Error() string { return e.Msg }

// NewToolError wraps an error message for ToolResult.IsError.
func NewToolError(format string, args ...any) error {
	return &ToolError{Msg: fmt.Sprintf(format, args...)}
}

// Tool is the agent-callable unit. Schema() is derived from the input
// struct's tags once at construction.
type Tool interface {
	Name() string
	Description() string
	Tier() ToolTier
	Schema() *jsonschema.Schema
	// Execute decodes + validates arguments into the tool's input struct,
	// then runs. Errors of type *ToolError flow to the model as results;
	// anything else aborts the turn.
	Execute(ctx context.Context, arguments json.RawMessage) (any, error)
}

// CallAwareTool is a Tool that also receives the full call (id included).
// The executor prefers ExecuteCall when implemented.
type CallAwareTool interface {
	Tool
	ExecuteCall(ctx context.Context, call ToolCall) (any, error)
}

type typedTool[I any] struct {
	name        string
	description string
	tier        ToolTier
	schema      *jsonschema.Schema
	run         func(ctx context.Context, input I) (any, error)
}

// NewTool defines a tool from its input struct + run function. The input
// struct's json/jsonschema tags are the single source of truth for the
// provider-facing schema; decoding the model's arguments into I is the
// validation step.
func NewTool[I any](name, description string, tier ToolTier, run func(ctx context.Context, input I) (any, error)) Tool {
	reflector := &jsonschema.Reflector{
		ExpandedStruct: true,
	}
	var zero I
	schema := reflector.Reflect(zero)
	return &typedTool[I]{
		name: name, description: description, tier: tier, schema: schema, run: run,
	}
}

func (t *typedTool[I]) Name() string               { return t.name }
func (t *typedTool[I]) Description() string        { return t.description }
func (t *typedTool[I]) Tier() ToolTier             { return t.tier }
func (t *typedTool[I]) Schema() *jsonschema.Schema { return t.schema }

func (t *typedTool[I]) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var input I
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &input); err != nil {
			return nil, NewToolError("Invalid arguments for %s: %v", t.name, err)
		}
	}
	return t.run(ctx, input)
}

// SchemaMap renders the schema as the JSON-serializable map providers expect
// ({"type":"object","properties":...,"required":[...]}).
func SchemaMap(t Tool) (map[string]any, error) {
	raw, err := json.Marshal(t.Schema())
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
