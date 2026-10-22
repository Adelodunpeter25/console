package visualize

import (
	"context"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type toolInput struct {
	Spec map[string]any `json:"spec" jsonschema:"required,description=Version 1 visual spec object. See the /visualize instructions for the grammar, limits, and examples."`
}

var visualizeTool = tools.NewTool(
	"visualize",
	"Render a native chart, metric, table, or text card in the chat from a versioned visual spec. Use the /visualize instructions for the spec grammar.",
	tools.TierRead,
	func(_ context.Context, in toolInput) (any, error) {
		canonical, spec, err := Compile(in.Spec)
		if err != nil {
			return nil, err
		}
		return tools.Envelope{
			Content: []map[string]any{{"type": "text", "text": Summary(spec)}},
			Visual: &tools.VisualArtifact{
				Version: SpecVersion,
				Spec:    canonical,
			},
		}, nil
	},
)

// NewTool returns the stateless visualize tool. It can be reused across runs;
// all state lives in the spec that comes back on the tool result.
func NewTool() tools.Tool { return visualizeTool }
