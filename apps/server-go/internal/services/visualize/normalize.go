package visualize

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Compile decodes, validates, normalizes, and returns the canonical spec bytes.
// Canonical means fixed struct field order, trimmed strings, and points sorted
// by x, so the same input always persists as the same bytes.
func Compile(input map[string]any) (json.RawMessage, *Spec, error) {
	spec, err := DecodeObject(input)
	if err != nil {
		return nil, nil, err
	}
	normalizeStrings(spec)
	if err := Validate(spec); err != nil {
		return nil, nil, err
	}
	sortPoints(spec)
	canonical, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, tools.NewToolError("spec: could not encode: %v", err)
	}
	if len(canonical) > MaxSpecBytes {
		return nil, nil, tools.NewToolError("spec: canonical size is %d bytes; limit is %d", len(canonical), MaxSpecBytes)
	}
	return canonical, spec, nil
}

func normalizeStrings(spec *Spec) {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Subtitle = strings.TrimSpace(spec.Subtitle)
	for i := range spec.Blocks {
		block := &spec.Blocks[i]
		switch block.Kind {
		case "text":
			if block.Text != nil {
				block.Text.Type = block.Kind
				block.Text.Text = strings.TrimSpace(block.Text.Text)
			}
		case "metric":
			if block.Metric != nil {
				block.Metric.Type = block.Kind
				block.Metric.Label = strings.TrimSpace(block.Metric.Label)
				block.Metric.Value = strings.TrimSpace(block.Metric.Value)
				block.Metric.Delta = trimOptional(block.Metric.Delta)
				block.Metric.Trend = strings.TrimSpace(block.Metric.Trend)
				block.Metric.Caption = trimOptional(block.Metric.Caption)
			}
		case "table":
			if block.Table != nil {
				block.Table.Type = block.Kind
				block.Table.Caption = trimOptional(block.Table.Caption)
				for j := range block.Table.Columns {
					block.Table.Columns[j] = strings.TrimSpace(block.Table.Columns[j])
				}
				for j := range block.Table.Rows {
					for k := range block.Table.Rows[j] {
						block.Table.Rows[j][k] = strings.TrimSpace(block.Table.Rows[j][k])
					}
				}
			}
		case "chart":
			if block.Chart != nil {
				block.Chart.Type = block.Kind
				block.Chart.Title = trimOptional(block.Chart.Title)
				block.Chart.XLabel = trimOptional(block.Chart.XLabel)
				block.Chart.YLabel = trimOptional(block.Chart.YLabel)
				for j := range block.Chart.Series {
					series := &block.Chart.Series[j]
					series.Name = strings.TrimSpace(series.Name)
					for k := range series.Points {
						series.Points[k].Label = trimOptional(series.Points[k].Label)
					}
				}
			}
		}
	}
}

func trimOptional(s string) string {
	if isEmptyOptional(s) {
		return ""
	}
	return strings.TrimSpace(s)
}

// isEmptyOptional drops whitespace-only optionals so they stay absent in the
// canonical spec.
func isEmptyOptional(s string) bool { return strings.TrimSpace(s) == "" }

func sortPoints(spec *Spec) {
	for i := range spec.Blocks {
		chart := spec.Blocks[i].Chart
		if chart == nil {
			continue
		}
		for j := range chart.Series {
			points := chart.Series[j].Points
			sort.SliceStable(points, func(a, b int) bool {
				if *points[a].X != *points[b].X {
					return *points[a].X < *points[b].X
				}
				if *points[a].Y != *points[b].Y {
					return *points[a].Y < *points[b].Y
				}
				return points[a].Label < points[b].Label
			})
		}
	}
}
