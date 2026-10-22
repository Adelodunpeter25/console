package visualize

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func object(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode test object: %v", err)
	}
	return out
}

func validChartJSON() string {
	return `{
		"version": 1,
		"title": "  Weekly signups  ",
		"blocks": [
			{"type":"metric","label":"MRR","value":"$42.1k","delta":"+8.2%","trend":"up"},
			{"type":"table","caption":"Top channels","columns":["Channel","Users"],"rows":[["Organic","640"]]},
			{"type":"chart","kind":"line","title":"Signups","xLabel":"Week","yLabel":"Users","series":[{"name":"This quarter","points":[{"x":2,"y":164,"label":"W3"},{"x":0,"y":120,"label":"W1"},{"x":1,"y":184,"label":"W2"}]}]}
		]
	}`
}

func TestSplitInvocation(t *testing.T) {
	cases := []struct {
		text string
		task string
		ok   bool
	}{
		{"/visualize chart our weekly signups", "chart our weekly signups", true},
		{"/visualize", "", true},
		{"/visualize   ", "", true},
		{"  /visualize\nmake a dashboard", "make a dashboard", true},
		{"/visualizer nope", "", false},
		{"/visualize2 nope", "", false},
		{"please /visualize this", "", false},
		{"visualize without the slash", "", false},
	}
	for _, tc := range cases {
		task, ok := SplitInvocation(tc.text)
		if task != tc.task || ok != tc.ok {
			t.Errorf("SplitInvocation(%q) = (%q, %v), want (%q, %v)", tc.text, task, ok, tc.task, tc.ok)
		}
	}
}

func TestCompileNormalizesAndSorts(t *testing.T) {
	canonical, spec, err := Compile(object(t, validChartJSON()))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if spec.Title != "Weekly signups" {
		t.Errorf("title = %q, want trimmed title", spec.Title)
	}
	chart := spec.Blocks[2].Chart
	if chart == nil {
		t.Fatal("chart block missing")
	}
	points := chart.Series[0].Points
	if len(points) != 3 {
		t.Fatalf("points = %d, want 3", len(points))
	}
	for i, want := range []float64{0, 1, 2} {
		if got := *points[i].X; got != want {
			t.Errorf("points[%d].x = %v, want %v", i, got, want)
		}
	}
	if !json.Valid(canonical) {
		t.Fatal("canonical spec is not valid JSON")
	}
	if got := Summary(spec); !strings.Contains(got, "Weekly signups") || !strings.Contains(got, "1 metric") {
		t.Errorf("summary = %q, want title and metric count", got)
	}
}

func TestCompileRejectsBadSpecs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"version", `{"version":2,"blocks":[{"type":"text","text":"x"}]}`, "version"},
		{"empty blocks", `{"version":1,"blocks":[]}`, "at least one block"},
		{"unknown block", `{"version":1,"blocks":[{"type":"pie","value":1}]}`, "unknown block type"},
		{"unknown root field", `{"version":1,"blocks":[{"type":"text","text":"x"}],"color":"red"}`, "unknown field"},
		{"unknown block field", `{"version":1,"blocks":[{"type":"metric","label":"x","value":"1","color":"red"}]}`, "unknown field"},
		{"bad trend", `{"version":1,"blocks":[{"type":"metric","label":"x","value":"1","trend":"sideways"}]}`, "trend"},
		{"table width", `{"version":1,"blocks":[{"type":"table","columns":["a","b"],"rows":[["1"]]}]}`, "expected 2 cells"},
		{"missing point x", `{"version":1,"blocks":[{"type":"chart","kind":"line","series":[{"name":"s","points":[{"y":1}]}]}]}`, "x and y are required"},
		{"duplicate x", `{"version":1,"blocks":[{"type":"chart","kind":"line","series":[{"name":"s","points":[{"x":1,"y":1},{"x":1,"y":2}]}]}]}`, "duplicate x"},
		{"bad kind", `{"version":1,"blocks":[{"type":"chart","kind":"pie","series":[{"name":"s","points":[{"x":1,"y":1}]}]}]}`, "want line, bar, area, or scatter"},
		{"duplicate series", `{"version":1,"blocks":[{"type":"chart","kind":"bar","series":[{"name":"s","points":[{"x":1,"y":1}]},{"name":"s","points":[{"x":2,"y":2}]}]}]}`, "duplicate series name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Compile(object(t, tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Compile error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCompileRejectsTooManyPoints(t *testing.T) {
	points := make([]map[string]any, 0, MaxPointsPerSeries+1)
	for i := 0; i <= MaxPointsPerSeries; i++ {
		points = append(points, map[string]any{"x": i, "y": i})
	}
	raw := map[string]any{
		"version": 1,
		"blocks": []any{map[string]any{
			"type":   "chart",
			"kind":   "line",
			"series": []any{map[string]any{"name": "s", "points": points}},
		}},
	}
	_, _, err := Compile(raw)
	if err == nil || !strings.Contains(err.Error(), "at most 2000 points") {
		t.Fatalf("Compile error = %v, want point-limit error", err)
	}
}

func TestToolReturnsVisualEnvelope(t *testing.T) {
	args, err := json.Marshal(map[string]any{"spec": object(t, validChartJSON())})
	if err != nil {
		t.Fatal(err)
	}
	out, err := NewTool().Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	envelope, ok := out.(tools.Envelope)
	if !ok {
		t.Fatalf("result type = %T, want tools.Envelope", out)
	}
	if envelope.Visual == nil || envelope.Visual.Version != 1 {
		t.Fatalf("visual = %#v, want version 1", envelope.Visual)
	}
	if !json.Valid(envelope.Visual.Spec) {
		t.Fatal("visual spec is not valid JSON")
	}
	if envelope.IsError {
		t.Fatal("visualize should not set IsError")
	}
	schema, err := json.Marshal(NewTool().Schema())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(schema), `"spec"`) {
		t.Fatalf("schema does not mention spec: %s", schema)
	}
}

func TestSkillHasGrammar(t *testing.T) {
	text := SkillText()
	for _, want := range []string{"visualize", "version", "chart", "metric", "table"} {
		if !strings.Contains(text, want) {
			t.Errorf("skill text missing %q", want)
		}
	}
}
