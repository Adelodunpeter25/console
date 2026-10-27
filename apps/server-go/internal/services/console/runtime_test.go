package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestRuntimeScriptMethodsAreInstalled(t *testing.T) {
	r, err := NewRuntime(nil, nil, "project", "")
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
