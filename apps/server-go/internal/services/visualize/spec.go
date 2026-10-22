// Package visualize owns the versioned visual spec and the lazy /visualize
// tool. The spec is data: it cannot execute code, fetch a remote asset, or set
// an absolute position. Native clients paint it with their own theme and DPI.
package visualize

import (
	"encoding/json"
	"fmt"
)

// SpecVersion is the only spec version this kernel accepts.
const SpecVersion = 1

// Spec is the root of one visual card.
type Spec struct {
	Version  int     `json:"version"`
	Title    string  `json:"title,omitempty"`
	Subtitle string  `json:"subtitle,omitempty"`
	Blocks   []Block `json:"blocks"`
}

// Block is one discriminated-union block. Exactly one of the payload fields is
// set; Kind names which one.
type Block struct {
	Kind   string
	Text   *TextBlock
	Metric *MetricBlock
	Table  *TableBlock
	Chart  *ChartBlock
}

// TextBlock is a short plain-text note.
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MetricBlock is one number with optional context.
type MetricBlock struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Delta   string `json:"delta,omitempty"`
	Trend   string `json:"trend,omitempty"`
	Caption string `json:"caption,omitempty"`
}

// TableBlock is a compact summary table.
type TableBlock struct {
	Type    string     `json:"type"`
	Caption string     `json:"caption,omitempty"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// ChartBlock is one chart with one or more series.
type ChartBlock struct {
	Type   string   `json:"type"`
	Kind   string   `json:"kind"`
	Title  string   `json:"title,omitempty"`
	XLabel string   `json:"xLabel,omitempty"`
	YLabel string   `json:"yLabel,omitempty"`
	Series []Series `json:"series"`
}

// Series is one named line, bar, area, or scatter series.
type Series struct {
	Name   string  `json:"name"`
	Points []Point `json:"points"`
}

// Point is one chart point. X and Y are pointers so a missing coordinate is a
// validation error rather than a silent zero.
type Point struct {
	X     *float64 `json:"x"`
	Y     *float64 `json:"y"`
	Label string   `json:"label,omitempty"`
}

// UnmarshalJSON decodes the block's type before selecting its payload. Unknown
// block types and unknown fields inside a block are rejected so a model mistake
// returns a useful tool error instead of disappearing.
func (b *Block) UnmarshalJSON(data []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	b.Kind = probe.Type
	switch probe.Type {
	case "text":
		var v TextBlock
		if err := decodeStrict(data, &v); err != nil {
			return err
		}
		b.Text = &v
	case "metric":
		var v MetricBlock
		if err := decodeStrict(data, &v); err != nil {
			return err
		}
		b.Metric = &v
	case "table":
		var v TableBlock
		if err := decodeStrict(data, &v); err != nil {
			return err
		}
		b.Table = &v
	case "chart":
		var v ChartBlock
		if err := decodeStrict(data, &v); err != nil {
			return err
		}
		b.Chart = &v
	default:
		return fmt.Errorf("unknown block type %q", probe.Type)
	}
	return nil
}

// MarshalJSON writes the selected payload as a flat object with its type
// discriminator, matching the public spec grammar.
func (b Block) MarshalJSON() ([]byte, error) {
	switch b.Kind {
	case "text":
		if b.Text == nil {
			return nil, fmt.Errorf("text block payload is missing")
		}
		return json.Marshal(b.Text)
	case "metric":
		if b.Metric == nil {
			return nil, fmt.Errorf("metric block payload is missing")
		}
		return json.Marshal(b.Metric)
	case "table":
		if b.Table == nil {
			return nil, fmt.Errorf("table block payload is missing")
		}
		return json.Marshal(b.Table)
	case "chart":
		if b.Chart == nil {
			return nil, fmt.Errorf("chart block payload is missing")
		}
		return json.Marshal(b.Chart)
	default:
		return nil, fmt.Errorf("unknown block type %q", b.Kind)
	}
}
