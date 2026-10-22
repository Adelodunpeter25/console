package visualize

import (
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

const (
	MaxSpecBytes          = 256 * 1024
	MaxBlocks             = 12
	MaxSeries             = 8
	MaxPointsPerSeries    = 2000
	MaxPointsTotal        = 5000
	MaxTableColumns       = 6
	MaxTableRows          = 30
	MaxTextRunes          = 1000
	MaxTitleRunes         = 160
	MaxSubtitleRunes      = 240
	MaxMetricLabelRunes   = 80
	MaxMetricValueRunes   = 80
	MaxMetricDeltaRunes   = 40
	MaxMetricCaptionRunes = 160
	MaxTableCaptionRunes  = 160
	MaxCellRunes          = 80
	MaxChartTitleRunes    = 160
	MaxAxisLabelRunes     = 80
	MaxSeriesNameRunes    = 80
	MaxPointLabelRunes    = 40
)

// Validate checks the parts that cannot be normalized away. The caller trims
// strings and sorts points first, so every limit here is measured on the final
// text.
func Validate(spec *Spec) error {
	if spec == nil {
		return tools.NewToolError("spec is required")
	}
	if spec.Version != SpecVersion {
		return tools.NewToolError("version: unsupported visual spec version %d (want %d)", spec.Version, SpecVersion)
	}
	if err := checkText("title", spec.Title, MaxTitleRunes, false); err != nil {
		return err
	}
	if err := checkText("subtitle", spec.Subtitle, MaxSubtitleRunes, false); err != nil {
		return err
	}
	if len(spec.Blocks) == 0 {
		return tools.NewToolError("blocks: at least one block is required")
	}
	if len(spec.Blocks) > MaxBlocks {
		return tools.NewToolError("blocks: at most %d blocks are allowed, got %d", MaxBlocks, len(spec.Blocks))
	}

	totalPoints := 0
	for i := range spec.Blocks {
		path := fmt.Sprintf("blocks[%d]", i)
		block := &spec.Blocks[i]
		switch block.Kind {
		case "text":
			if block.Text == nil {
				return tools.NewToolError("%s.text: payload is missing", path)
			}
			if err := checkText(path+".text", block.Text.Text, MaxTextRunes, true); err != nil {
				return err
			}
		case "metric":
			if block.Metric == nil {
				return tools.NewToolError("%s.metric: payload is missing", path)
			}
			if err := checkMetric(path, block.Metric); err != nil {
				return err
			}
		case "table":
			if block.Table == nil {
				return tools.NewToolError("%s.table: payload is missing", path)
			}
			if err := checkTable(path, block.Table); err != nil {
				return err
			}
		case "chart":
			if block.Chart == nil {
				return tools.NewToolError("%s.chart: payload is missing", path)
			}
			n, err := checkChart(path, block.Chart)
			if err != nil {
				return err
			}
			totalPoints += n
		default:
			return tools.NewToolError("%s.type: unknown block type %q", path, block.Kind)
		}
	}
	if totalPoints > MaxPointsTotal {
		return tools.NewToolError("blocks: at most %d chart points are allowed, got %d", MaxPointsTotal, totalPoints)
	}
	return nil
}

func checkMetric(path string, m *MetricBlock) error {
	if err := checkText(path+".label", m.Label, MaxMetricLabelRunes, true); err != nil {
		return err
	}
	if err := checkText(path+".value", m.Value, MaxMetricValueRunes, true); err != nil {
		return err
	}
	if err := checkText(path+".delta", m.Delta, MaxMetricDeltaRunes, false); err != nil {
		return err
	}
	if err := checkText(path+".caption", m.Caption, MaxMetricCaptionRunes, false); err != nil {
		return err
	}
	switch m.Trend {
	case "", "up", "down", "flat":
	default:
		return tools.NewToolError("%s.trend: want up, down, flat, or empty; got %q", path, m.Trend)
	}
	return nil
}

func checkTable(path string, t *TableBlock) error {
	if err := checkText(path+".caption", t.Caption, MaxTableCaptionRunes, false); err != nil {
		return err
	}
	if len(t.Columns) == 0 {
		return tools.NewToolError("%s.columns: at least one column is required", path)
	}
	if len(t.Columns) > MaxTableColumns {
		return tools.NewToolError("%s.columns: at most %d columns are allowed, got %d", path, MaxTableColumns, len(t.Columns))
	}
	for i, col := range t.Columns {
		if err := checkText(fmt.Sprintf("%s.columns[%d]", path, i), col, MaxCellRunes, true); err != nil {
			return err
		}
	}
	if len(t.Rows) == 0 {
		return tools.NewToolError("%s.rows: at least one row is required", path)
	}
	if len(t.Rows) > MaxTableRows {
		return tools.NewToolError("%s.rows: at most %d rows are allowed, got %d", path, MaxTableRows, len(t.Rows))
	}
	for i, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return tools.NewToolError("%s.rows[%d]: expected %d cells, got %d", path, i, len(t.Columns), len(row))
		}
		for j, cell := range row {
			if err := checkText(fmt.Sprintf("%s.rows[%d][%d]", path, i, j), cell, MaxCellRunes, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkChart(path string, c *ChartBlock) (int, error) {
	switch c.Kind {
	case "line", "bar", "area", "scatter":
	default:
		return 0, tools.NewToolError("%s.kind: want line, bar, area, or scatter; got %q", path, c.Kind)
	}
	if err := checkText(path+".title", c.Title, MaxChartTitleRunes, false); err != nil {
		return 0, err
	}
	if err := checkText(path+".xLabel", c.XLabel, MaxAxisLabelRunes, false); err != nil {
		return 0, err
	}
	if err := checkText(path+".yLabel", c.YLabel, MaxAxisLabelRunes, false); err != nil {
		return 0, err
	}
	if len(c.Series) == 0 {
		return 0, tools.NewToolError("%s.series: at least one series is required", path)
	}
	if len(c.Series) > MaxSeries {
		return 0, tools.NewToolError("%s.series: at most %d series are allowed, got %d", path, MaxSeries, len(c.Series))
	}

	names := make(map[string]bool, len(c.Series))
	total := 0
	for i, series := range c.Series {
		sp := fmt.Sprintf("%s.series[%d]", path, i)
		if err := checkText(sp+".name", series.Name, MaxSeriesNameRunes, true); err != nil {
			return 0, err
		}
		if names[series.Name] {
			return 0, tools.NewToolError("%s.name: duplicate series name %q", sp, series.Name)
		}
		names[series.Name] = true
		if len(series.Points) == 0 {
			return 0, tools.NewToolError("%s.points: at least one point is required", sp)
		}
		if len(series.Points) > MaxPointsPerSeries {
			return 0, tools.NewToolError("%s.points: at most %d points are allowed, got %d", sp, MaxPointsPerSeries, len(series.Points))
		}
		total += len(series.Points)
		xs := make(map[float64]bool, len(series.Points))
		for j, point := range series.Points {
			pp := fmt.Sprintf("%s.points[%d]", sp, j)
			if point.X == nil || point.Y == nil {
				return 0, tools.NewToolError("%s: x and y are required", pp)
			}
			if !finite(*point.X) || !finite(*point.Y) {
				return 0, tools.NewToolError("%s: x and y must be finite numbers", pp)
			}
			if xs[*point.X] {
				return 0, tools.NewToolError("%s.x: duplicate x value %v", pp, *point.X)
			}
			xs[*point.X] = true
			if err := checkText(pp+".label", point.Label, MaxPointLabelRunes, false); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func checkText(path, value string, max int, required bool) error {
	if required && value == "" {
		return tools.NewToolError("%s: is required", path)
	}
	if utf8.RuneCountInString(value) > max {
		return tools.NewToolError("%s: must be %d characters or fewer", path, max)
	}
	for _, r := range value {
		if r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return tools.NewToolError("%s: contains an unsupported control character", path)
		}
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
