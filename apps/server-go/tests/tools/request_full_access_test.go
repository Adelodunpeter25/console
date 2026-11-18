package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func runRequestFullAccess(t *testing.T, handler tools.AskHandler, switched *bool) string {
	return runRequestFullAccessMode(t, handler, switched, true)
}

func runRequestFullAccessMode(t *testing.T, handler tools.AskHandler, switched *bool, planMode bool) string {
	t.Helper()
	tool := tools.NewRequestFullAccessTool(handler, func() error { *switched = true; return nil }, func() bool { return planMode })
	args, _ := json.Marshal(map[string]any{"reason": "implement the plan"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return resultText(t, out)
}

func TestRequestFullAccessApproveSwitches(t *testing.T) {
	var asked tools.AskQuestionRequest
	var switched bool
	text := runRequestFullAccess(t, func(_ context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
		asked = req
		return tools.AskAnswer{Text: tools.FullAccessApprove}, nil
	}, &switched)
	if !switched || !strings.Contains(text, "approved") {
		t.Fatalf("switched=%v text=%q", switched, text)
	}
	if asked.Skippable || !strings.Contains(asked.Question, "implement the plan") {
		t.Fatalf("unexpected ask: %+v", asked)
	}
}

func TestRequestFullAccessDenyOrSkipStays(t *testing.T) {
	for _, answer := range []string{tools.FullAccessDeny, ""} {
		var switched bool
		text := runRequestFullAccess(t, func(context.Context, tools.AskQuestionRequest) (tools.AskAnswer, error) {
			return tools.AskAnswer{Text: answer}, nil
		}, &switched)
		if switched || !strings.Contains(text, "declined") {
			t.Fatalf("answer %q: switched=%v text=%q", answer, switched, text)
		}
	}
}

func TestRequestFullAccessDenyWithFeedback(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Deny: write more unit tests first", "write more unit tests first"},
		{"Deny - please outline the schema", "please outline the schema"},
		{"stay in plan mode and check edge cases", "stay in plan mode and check edge cases"},
	}
	for _, tc := range cases {
		var switched bool
		text := runRequestFullAccess(t, func(context.Context, tools.AskQuestionRequest) (tools.AskAnswer, error) {
			return tools.AskAnswer{Text: tc.input}, nil
		}, &switched)
		if switched {
			t.Fatalf("must not switch on deny with feedback %q", tc.input)
		}
		expectedPhrase := fmt.Sprintf("with feedback: %q", tc.expected)
		if !strings.Contains(text, expectedPhrase) {
			t.Fatalf("text %q does not contain expected feedback %q", text, tc.expected)
		}
		if !strings.Contains(text, "address this feedback, and continue planning") {
			t.Fatalf("text %q does not contain instructions to address feedback and continue planning", text)
		}
	}
}

func TestRequestFullAccessHeadlessNeverSwitches(t *testing.T) {
	var switched bool
	runRequestFullAccess(t, nil, &switched)
	if switched {
		t.Fatal("headless run must not upgrade")
	}
}

func TestRequestFullAccessOutsidePlanModeAnswersDirectly(t *testing.T) {
	var switched bool
	var asked bool
	text := runRequestFullAccessMode(t, func(context.Context, tools.AskQuestionRequest) (tools.AskAnswer, error) {
		asked = true
		return tools.AskAnswer{Text: tools.FullAccessApprove}, nil
	}, &switched, false)
	if switched || asked {
		t.Fatalf("non-plan call must not ask or switch: switched=%v asked=%v", switched, asked)
	}
	if !strings.Contains(text, "Full Access") {
		t.Fatalf("text = %q", text)
	}
}

func TestExecutorSetModeLiftsPlanDenial(t *testing.T) {
	var ran bool
	exec := tools.NewTool("dangerous_exec", "x", tools.TierExec, func(context.Context, struct{}) (any, error) {
		ran = true
		return []map[string]any{{"type": "text", "text": "ok"}}, nil
	})
	e := loop.NewExecutor(tools.NewRegistry(exec), permissions.PlanMode)
	call := tools.ToolCall{ID: "1", Name: "dangerous_exec", Arguments: []byte(`{}`)}
	if res, _ := e.Execute(context.Background(), call); !res.IsError || ran {
		t.Fatal("plan mode must deny exec")
	}
	e.SetMode(permissions.FullAccess)
	if res, _ := e.Execute(context.Background(), call); res.IsError || !ran {
		t.Fatalf("full access must run exec: %+v", res)
	}
}
