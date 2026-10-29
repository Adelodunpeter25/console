package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRuntimeScriptMethodsAreInstalled(t *testing.T) {
	r, err := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, "project", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Eval(context.Background(), `console.scripts.list()`, time.Second)
	if err == nil {
		t.Fatal("expected unavailable scripts service error")
	}
}

func TestMarshalResult(t *testing.T) {
	result, err := marshalResult(map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(result.StructuredContent, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["ok"] != true {
		t.Fatalf("decoded result = %#v", decoded)
	}
}

func TestApprovalModesListsClampOrder(t *testing.T) {
	runtime, err := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	payload := evalJSON(t, runtime, `console.approvalModes()`).(map[string]any)
	ordering, ok := payload["ordering"].([]any)
	if !ok || len(ordering) != 4 {
		t.Fatalf("ordering = %#v", payload["ordering"])
	}
	for i, want := range []string{"plan-mode", "always-ask", "accept-edits", "full-access"} {
		if ordering[i] != want {
			t.Fatalf("ordering[%d] = %v, want %q", i, ordering[i], want)
		}
	}
	modes, ok := payload["modes"].([]any)
	if !ok || len(modes) != 4 {
		t.Fatalf("modes = %#v", payload["modes"])
	}
	first, _ := modes[0].(map[string]any)
	if first["mode"] != "plan-mode" || first["rank"] != float64(0) {
		t.Fatalf("first mode = %#v", first)
	}
	if description, _ := first["description"].(string); description == "" {
		t.Fatal("plan-mode needs a description")
	}
}
