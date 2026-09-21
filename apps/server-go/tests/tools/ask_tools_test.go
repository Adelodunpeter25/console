// Coverage for the ask / askMany tools: headless default-option fallback
// and handler-driven interactive answers.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestAskHeadlessWithOptions(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"question": "Pick one", "options": []string{"a", "b"}})
	out, err := tools.Ask.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Pick one") || !strings.Contains(s, `"a"`) {
		t.Fatalf("output: %v", out)
	}
}

func TestAskHeadlessNoOptions(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"question": "Free text?"})
	out, err := tools.Ask.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "skipped") {
		t.Fatalf("output: %v", out)
	}
}

func TestAskMissingQuestion(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"question": ""})
	if _, err := tools.Ask.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for missing question")
	}
}

func TestAskWithHandler(t *testing.T) {
	var captured tools.AskQuestionRequest
	handler := func(ctx context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
		captured = req
		return tools.AskAnswer{Text: "yes"}, nil
	}
	ask := tools.NewAskTool(handler)
	args, _ := json.Marshal(map[string]any{"question": "Continue?", "skippable": false})
	out, err := ask.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Question != "Continue?" || captured.Skippable {
		t.Fatalf("captured request: %+v", captured)
	}
	s := resultText(t, out)
	if !strings.Contains(s, `"yes"`) {
		t.Fatalf("output: %v", out)
	}
}

func TestAskManyHeadless(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"questions": []map[string]any{
			{"question": "Q1"},
			{"question": "Q2", "options": []string{"x"}},
		},
	})
	out, err := tools.AskMany.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Q1") || !strings.Contains(s, "Q2") || !strings.Contains(s, `"x"`) {
		t.Fatalf("output: %v", out)
	}
}

func TestAskManyWithHandlerSharesBatchID(t *testing.T) {
	var batchIDs []string
	handler := func(ctx context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
		batchIDs = append(batchIDs, req.BatchID)
		return tools.AskAnswer{Text: "ok"}, nil
	}
	askMany := tools.NewAskManyTool(handler)
	args, _ := json.Marshal(map[string]any{
		"questions": []map[string]any{{"question": "Q1"}, {"question": "Q2"}},
	})
	out, err := askMany.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(batchIDs) != 2 || batchIDs[0] == "" || batchIDs[0] != batchIDs[1] {
		t.Fatalf("batch ids: %v", batchIDs)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Q1:") || !strings.Contains(s, "Q2:") {
		t.Fatalf("output: %v", out)
	}
}

func TestAskManyMissingQuestions(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"questions": []map[string]any{}})
	if _, err := tools.AskMany.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for empty questions")
	}
}
