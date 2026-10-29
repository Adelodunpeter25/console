package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRuntimeScriptMethodsAreInstalled(t *testing.T) {
	r, err := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, QueueControl{}, "project", "")
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
	runtime, err := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, QueueControl{}, "", "")
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

// The script engine hands whole numbers over as int64, so every integer type
// must be accepted. A regression here made console.ports.forward reject every
// port, including plain literals like 3000.
func TestPortArgAcceptsEveryIntegerType(t *testing.T) {
	for _, value := range []any{int(3000), int32(3000), int64(3000), float64(3000)} {
		got, err := portArg(map[string]any{"port": value})
		if err != nil || got != 3000 {
			t.Errorf("portArg(%T) = %d, %v", value, got, err)
		}
	}
	for name, value := range map[string]any{
		"fraction": 3000.5, "string": "3000", "zero": int64(0), "negative": int64(-1), "too big": int64(70000), "bool": true,
	} {
		if _, err := portArg(map[string]any{"port": value}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := portArg(map[string]any{}); err == nil {
		t.Error("a missing port must be an error")
	}
}

func TestOptionalIntRejectsWrongTypes(t *testing.T) {
	n, ok, err := optionalInt(map[string]any{"limit": int64(5)}, "limit")
	if err != nil || !ok || n != 5 {
		t.Fatalf("int64 limit = %d %v %v", n, ok, err)
	}
	if _, ok, err := optionalInt(map[string]any{}, "limit"); ok || err != nil {
		t.Fatalf("absent limit = %v %v", ok, err)
	}
	for _, bad := range []any{"5", 5.5, true} {
		if _, _, err := optionalInt(map[string]any{"limit": bad}, "limit"); err == nil {
			t.Errorf("%T %v must be rejected, not ignored", bad, bad)
		}
	}
}
