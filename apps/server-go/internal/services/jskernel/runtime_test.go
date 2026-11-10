package jskernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestKernelBasicEvalAndPersistence(t *testing.T) {
	kernel, err := New(Options{Root: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = kernel.Eval(context.Background(), "var a = 40;", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	res, err := kernel.Eval(context.Background(), "a + 2;", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parts := res.([]map[string]any)
	if len(parts) == 0 || parts[0]["text"] != "42" {
		t.Fatalf("expected 42, got %#v", res)
	}
}

func TestKernelDottedMethodsAndGuard(t *testing.T) {
	methods := []Method{
		{
			Name: "math.add",
			Call: func(ctx context.Context, args map[string]any) (*Result, error) {
				a := args["a"].(int64)
				b := args["b"].(int64)
				raw, _ := json.Marshal(map[string]any{"sum": a + b})
				return &Result{StructuredContent: raw}, nil
			},
		},
	}
	kernel, err := New(Options{Root: "api", PlainResults: true}, methods)
	if err != nil {
		t.Fatal(err)
	}

	// Valid call
	res, err := kernel.Eval(context.Background(), "api.math.add({a: 10, b: 20})", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parts := res.([]map[string]any)
	if len(parts) == 0 || !strings.Contains(parts[0]["text"].(string), `"sum":30`) {
		t.Fatalf("expected sum 30, got %#v", res)
	}

	// Invalid method call: api.math.sub()
	_, err = kernel.Eval(context.Background(), "api.math.sub()", time.Second)
	if err == nil {
		t.Fatal("expected error on unknown method")
	}
	if !strings.Contains(err.Error(), "api.math.sub does not exist") || !strings.Contains(err.Error(), "add") {
		t.Fatalf("expected guard message listing available methods, got: %v", err)
	}
}

func TestKernelAsyncAndTopLevelAwait(t *testing.T) {
	kernel, err := New(Options{Root: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Top-level await expression
	res, err := kernel.Eval(context.Background(), "await Promise.resolve(100)", time.Second)
	if err != nil {
		t.Fatalf("top level await failed: %v", err)
	}
	parts := res.([]map[string]any)
	if len(parts) == 0 || parts[0]["text"] != "100" {
		t.Fatalf("expected 100, got %#v", res)
	}

	// Multiple statements with await
	code := `
		const a = await Promise.resolve(10);
		const b = await Promise.resolve(25);
		a + b;
	`
	res2, err := kernel.Eval(context.Background(), code, time.Second)
	if err != nil {
		t.Fatalf("multiline await failed: %v", err)
	}
	parts2 := res2.([]map[string]any)
	if len(parts2) == 0 || parts2[0]["text"] != "35" {
		t.Fatalf("expected 35, got %#v", res2)
	}
}

func TestKernelSetTimeout(t *testing.T) {
	kernel, err := New(Options{Root: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	code := `
		new Promise(resolve => {
			setTimeout(() => resolve("delayed"), 15);
		});
	`
	res, err := kernel.Eval(context.Background(), code, time.Second)
	if err != nil {
		t.Fatalf("setTimeout promise failed: %v", err)
	}
	parts := res.([]map[string]any)
	if len(parts) == 0 || parts[0]["text"] != `"delayed"` {
		t.Fatalf("expected delayed, got %#v", res)
	}
}

func TestKernelPrintOutput(t *testing.T) {
	kernel, err := New(Options{Root: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	code := `
		print("hello", 123);
		"result"
	`
	res, err := kernel.Eval(context.Background(), code, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	parts := res.([]map[string]any)
	expected := "hello 123\n\"result\""
	if len(parts) == 0 || parts[0]["text"] != expected {
		t.Fatalf("expected %q, got %#v", expected, res)
	}
}

func TestKernelTimeoutInterrupt(t *testing.T) {
	kernel, err := New(Options{Root: "test", InterruptMessage: "stopped in time"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = kernel.Eval(context.Background(), "for (;;) {}", 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected loop to time out")
	}
	if !strings.Contains(err.Error(), "stopped in time") {
		t.Fatalf("expected interrupt message, got: %v", err)
	}
}
