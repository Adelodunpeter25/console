// The run-script engine: references, interpolation and element matching. All
// pure logic, so no driver library is needed and CI covers it fully.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/commands"
)

func scriptScope() map[string]any {
	return map[string]any{
		"launch": map[string]any{"pid": float64(28468)},
		"win":    map[string]any{"window_id": float64(34345), "title": "Calculator"},
		"keys": map[string]any{"elements": []any{
			map[string]any{"label": "seven", "role": "button", "element_token": "tok7"},
			map[string]any{"label": "multiply", "role": "button", "element_token": "tokX"},
		}},
	}
}

func TestResolveScriptPath(t *testing.T) {
	scope := scriptScope()
	cases := []struct {
		path string
		want any
	}{
		{"launch.pid", float64(28468)},
		{"win.window_id", float64(34345)},
		{"win.title", "Calculator"},
		{"keys.elements.1.label", "multiply"},
	}
	for _, c := range cases {
		got, err := commands.ResolveScriptPath(scope, c.path)
		if err != nil {
			t.Errorf("%s: %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.path, got, c.want)
		}
	}
}

func TestResolveScriptPathErrorsNameTheProblem(t *testing.T) {
	scope := scriptScope()
	for _, path := range []string{"missing.pid", "launch.nope", "keys.elements.9", "win.title.deeper"} {
		if _, err := commands.ResolveScriptPath(scope, path); err == nil {
			t.Errorf("%s: expected an error", path)
		}
	}
}

func TestInterpolateWholeReferenceKeepsType(t *testing.T) {
	scope := scriptScope()
	// A whole-string reference must stay a number: Cua expects window_id as a
	// JSON number, and a stringified id would fail its schema.
	got, err := commands.InterpolateScriptValue("{{win.window_id}}", scope)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if got != float64(34345) {
		t.Fatalf("got %v (%T), want a number", got, got)
	}
}

func TestInterpolateEmbeddedReferenceBecomesText(t *testing.T) {
	scope := scriptScope()
	got, err := commands.InterpolateScriptValue("pid-{{launch.pid}}", scope)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	if got != "pid-28468" {
		t.Fatalf("got %q", got)
	}
}

func TestInterpolateRecursesAndReportsBadPaths(t *testing.T) {
	scope := scriptScope()
	args := map[string]any{
		"pid":       "{{launch.pid}}",
		"window_id": "{{win.window_id}}",
		"nested":    map[string]any{"token": "{{keys.elements.0.element_token}}"},
		"label":     "press {{keys.elements.1.label}}",
	}
	got, err := commands.InterpolateScriptValue(args, scope)
	if err != nil {
		t.Fatalf("interpolate: %v", err)
	}
	resolved := got.(map[string]any)
	if resolved["pid"] != float64(28468) {
		t.Errorf("pid = %v", resolved["pid"])
	}
	if resolved["nested"].(map[string]any)["token"] != "tok7" {
		t.Errorf("nested token = %v", resolved["nested"])
	}
	if resolved["label"] != "press multiply" {
		t.Errorf("label = %v", resolved["label"])
	}

	if _, err := commands.InterpolateScriptValue("{{nope.x}}", scope); err == nil {
		t.Error("expected an error for an unknown capture")
	}
}

func TestFindScriptElement(t *testing.T) {
	elements := scriptScope()["keys"].(map[string]any)["elements"].([]any)
	element, err := commands.FindScriptElement(elements, map[string]any{"label": "seven"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if element["element_token"] != "tok7" {
		t.Errorf("token = %v", element["element_token"])
	}
}

func TestFindScriptElementMissListsLabels(t *testing.T) {
	elements := scriptScope()["keys"].(map[string]any)["elements"].([]any)
	_, err := commands.FindScriptElement(elements, map[string]any{"label": "nine"})
	if err == nil {
		t.Fatal("expected a miss")
	}
	// The error must show what IS there, because a mistyped label is the
	// usual cause and the transcript is the only thing the author sees.
	for _, want := range []string{"seven", "multiply"} {
		if !containsStr(err.Error(), want) {
			t.Errorf("error %q does not list %q", err, want)
		}
	}
	if _, err := commands.FindScriptElement(elements, map[string]any{}); err == nil {
		t.Error("an empty match must be rejected, not match the first element")
	}
}

func containsStr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestScriptStepMeasuresElapsed pins the transcript clock: a step that sleeps
// must report it. $sleep touches no driver, so no library is needed. The bound
// is generous (half the sleep) because CI machines stall, but a zero Ms from a
// 150ms sleep means the deferred write is landing on a discarded copy.
func TestScriptStepMeasuresElapsed(t *testing.T) {
	entry := commands.ExecuteScriptStep(nil, map[string]any{}, 0, commands.ScriptStep{
		Label: "nap",
		Tool:  "$sleep",
		Args:  map[string]any{"ms": float64(150)},
	})
	if !entry.OK {
		t.Fatalf("sleep step failed: %s", entry.Error)
	}
	if entry.Ms < 75 {
		t.Fatalf("Ms = %d after a 150ms sleep; the clock is not landing on the returned entry", entry.Ms)
	}
}
