// Compaction coverage: token estimates, shake ceilings, cut points,
// structural summaries, history rewriting, overflow detection, and the
// loop emergency-retry path.
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/compaction"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func compactionHistory() []any {
	return []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "first task"},
		loop.AssistantMessage{Role: loop.RoleAssistant, ID: "m1", Content: []any{
			loop.TextPart{Type: "text", Text: "reading"},
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c1", Name: "read_file", Arguments: jsonRaw(`{"path":"a.go"}`)}},
		}, StopReason: loop.StopToolUse},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "c1", ToolName: "read_file", Content: "package main"},
		}},
		loop.UserMessage{Role: loop.RoleUser, Content: "second task"},
		loop.AssistantMessage{Role: loop.RoleAssistant, ID: "m2", Content: []any{
			loop.TextPart{Type: "text", Text: "done"},
		}, StopReason: loop.StopStop},
	}
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }

func TestEstimateTokens(t *testing.T) {
	history := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "12345678"},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.TextPart{Type: "text", Text: "1234"},
		}},
	}
	// 12 chars prose → ceil(12/4) = 3.
	if got := compaction.EstimateMessageTokens(history); got != 3 {
		t.Fatalf("tokens: %d", got)
	}
	withImage := []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi", Attachments: []loop.ImageAttachment{{Data: "x", MimeType: "image/png"}}}}
	if got := compaction.EstimateMessageTokens(withImage); got != 1001 {
		t.Fatalf("image tokens: %d", got)
	}
	est := compaction.EstimatePayloadTokens(history, "sys", []tools.Definition{{Name: "t", Description: "d"}})
	if est.Source != "local" || est.Tokens < 2000 {
		t.Fatalf("payload: %+v", est)
	}
}

func TestShakeConversation(t *testing.T) {
	big := strings.Repeat("x", 9000)
	history := []any{
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "c1", Content: big},
			{ToolCallID: "c2", Content: "short", IsError: true},
		}},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "c3", Content: big},
		}},
	}
	// Protected suffix (index ≥ 1) untouched without emergency.
	shaken := compaction.ShakeConversation(history, 8000, 1, false)
	first := shaken[0].(loop.ToolResultMessage)
	if s, _ := first.Results[0].Content.(string); !strings.HasSuffix(s, "…]") && !strings.Contains(s, "shaken") {
		t.Fatalf("must truncate: %.40s", s)
	}
	second := shaken[1].(loop.ToolResultMessage)
	if s, _ := second.Results[0].Content.(string); len(s) != 9000 {
		t.Fatal("protected suffix must be untouched")
	}
	// Emergency truncates everywhere.
	emergency := compaction.ShakeConversation(history, 2000, len(history), true)
	for _, m := range emergency {
		for _, r := range m.(loop.ToolResultMessage).Results {
			if s, ok := r.Content.(string); ok && !r.IsError && len(s) > 2100 {
				t.Fatalf("emergency must truncate: %d", len(s))
			}
		}
	}
	// Originals never mutated.
	if s, _ := history[0].(loop.ToolResultMessage).Results[0].Content.(string); len(s) != 9000 {
		t.Fatal("shake must not mutate inputs")
	}
}

func TestProtectedRecentStart(t *testing.T) {
	history := compactionHistory()
	if got := compaction.ProtectedRecentStart(history, 1); got != 3 {
		t.Fatalf("recent start: %d", got)
	}
	if got := compaction.ProtectedRecentStart(history, 5); got != 0 {
		t.Fatalf("few turns: %d", got)
	}
}

func TestIsToolCallSafe(t *testing.T) {
	history := compactionHistory()
	if !compaction.IsToolCallSafe(history, 3) {
		t.Fatal("clean user boundary must be safe")
	}
	if compaction.IsToolCallSafe(history, 2) {
		t.Fatal("cut at toolResult must be unsafe")
	}
	// Orphan: keep a toolResult whose call was discarded.
	orphan := []any{
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c9", Name: "t"}},
		}},
		loop.UserMessage{Role: loop.RoleUser, Content: "next"},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{{ToolCallID: "c9"}}},
	}
	if compaction.IsToolCallSafe(orphan, 1) {
		t.Fatal("orphaned result must be unsafe")
	}
}

func TestFindCutPoint(t *testing.T) {
	short := compactionHistory()[:3]
	if cut := compaction.FindCutPoint(short, 40000, 3); cut.FirstKeptIndex != 0 {
		t.Fatalf("short: %+v", cut)
	}
	cut := compaction.FindCutPoint(compactionHistory(), 1, 1)
	if !cut.IsUserBoundary || cut.FirstKeptIndex <= 0 {
		t.Fatalf("cut: %+v", cut)
	}
}

func TestBuildStructuralSummary(t *testing.T) {
	summary := compaction.BuildStructuralSummary(compactionHistory())
	for _, want := range []string{
		"Conversation Checkpoint", "first task", "read_file", "<files>", "a.go",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %q in:\n%s", want, summary)
		}
	}
	if got := compaction.BuildStructuralSummary(nil); !strings.Contains(got, "preliminary exploratory") {
		t.Fatalf("empty: %q", got)
	}
}

func TestCompactHistoryThreshold(t *testing.T) {
	history := compactionHistory()
	opts := compaction.Options{TokenThreshold: 1}
	if !compaction.ShouldCompact(history, 272_000, opts, "", nil) {
		t.Fatal("tiny threshold must trigger")
	}
	if compaction.ShouldCompact(history, 272_000, opts, "", nil) != true {
		t.Fatal("unreachable")
	}
	disabled := compaction.Options{TokenThreshold: 1}
	off := false
	disabled.Enabled = &off
	if compaction.ShouldCompact(history, 272_000, disabled, "", nil) {
		t.Fatal("disabled must not trigger")
	}
	if compaction.ShouldCompact(history, 0, opts, "", nil) {
		t.Fatal("zero window must not trigger")
	}
	result := compaction.CompactHistory(history, opts)
	if result.TokensAfter > result.TokensBefore {
		t.Fatalf("compaction must not grow tokens: %+v", result)
	}
	if len(result.CompactedMessages) != len(history) {
		t.Fatalf("small history should remain intact: %d -> %d", len(history), len(result.CompactedMessages))
	}
	if !strings.Contains(result.Summary, "skipped") {
		t.Fatalf("expected compaction skip reason: %q", result.Summary)
	}
	// Token drop on a history dominated by one huge tool result.
	big := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "go"},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "c1", Name: "read_file"}},
		}, StopReason: loop.StopToolUse},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "c1", Content: strings.Repeat("z", 100_000)},
		}},
		loop.UserMessage{Role: loop.RoleUser, Content: "again"},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.TextPart{Type: "text", Text: "ok"},
		}, StopReason: loop.StopStop},
	}
	bigResult := compaction.CompactHistory(big, compaction.Options{TokenThreshold: 1})
	if bigResult.TokensAfter >= bigResult.TokensBefore {
		t.Fatalf("tokens must drop: %+v", bigResult)
	}
	withSummary := compaction.CompactHistoryWithSummary(big, opts, "custom summary")
	if withSummary.Summary != "custom summary" {
		t.Fatalf("summary: %+v", withSummary)
	}
}

func TestPackSummaryInput(t *testing.T) {
	packed := compaction.PackSummaryInput(compactionHistory(), 60_000)
	if len(packed) != 1 {
		t.Fatalf("packed: %+v", packed)
	}
	user, ok := packed[0].(loop.UserMessage)
	if !ok || !strings.Contains(user.Content, "[user]") {
		t.Fatalf("packed content: %+v", packed[0])
	}
	tiny := compaction.PackSummaryInput(compactionHistory(), 10)
	if text := tiny[0].(loop.UserMessage).Content; !strings.Contains(text, "omitted for length") {
		t.Fatalf("budget: %q", text)
	}
}

func TestIsContextOverflowError(t *testing.T) {
	if !compaction.IsContextOverflowError(errors.New("maximum context length exceeded")) {
		t.Fatal("pattern must match")
	}
	if !compaction.IsContextOverflowError(errors.New("request failed (400): too many tokens")) {
		t.Fatal("400+keyword must match")
	}
	if compaction.IsContextOverflowError(errors.New("request failed (400): invalid auth")) {
		t.Fatal("auth 400 must not match")
	}
	if compaction.IsContextOverflowError(nil) {
		t.Fatal("nil must not match")
	}
}

type overflowProvider struct {
	calls int
}

func (p *overflowProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	p.calls++
	if p.calls == 1 {
		err := errors.New("Codex request failed (400): maximum context length exceeded")
		s.Fail(err)
		return err
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: "recovered"})
	s.Complete()
	return nil
}

func TestLoopEmergencyRetry(t *testing.T) {
	provider := &overflowProvider{}
	executor := loop.NewExecutor(tools.NewRegistry(tools.DefaultTools()...), "full-access", helpers.AutoApprover{})
	agent := loop.New(provider, executor, nil)
	agent.Compaction = &loop.CompactionHooks{
		IsOverflow: compaction.IsContextOverflowError,
		Emergency: func(history []any) []any {
			return history[:1]
		},
	}
	events, err := agent.Run(context.Background(), "sess", "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	sawText := false
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			break
		}
		if event.Kind == loop.EventText && event.Text == "recovered" {
			sawText = true
		}
	}
	if !sawText {
		t.Fatal("retry turn never ran")
	}
	if provider.calls != 2 {
		t.Fatalf("calls: %d", provider.calls)
	}
}

func TestLoopNoRetryOnPlainError(t *testing.T) {
	failing := &failingProvider{err: errors.New("boom: invalid auth (400)")}
	executor := loop.NewExecutor(tools.NewRegistry(), "full-access", helpers.AutoApprover{})
	agent := loop.New(failing, executor, nil)
	agent.Compaction = &loop.CompactionHooks{
		IsOverflow: compaction.IsContextOverflowError,
		Emergency:  func(history []any) []any { return history },
	}
	events, err := agent.Run(context.Background(), "sess", "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, err, ok := events.Next()
		if !ok {
			if err == nil {
				t.Fatal("expected the auth error")
			}
			return
		}
	}
}

type failingProvider struct{ err error }

func (p *failingProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	s.Fail(p.err)
	return p.err
}

type messageRecorder struct {
	sizes []int
	mock  *helpers.MockProvider
}

func (r *messageRecorder) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	total := 0
	for _, m := range req.Messages {
		raw, _ := json.Marshal(m)
		total += len(raw)
	}
	r.sizes = append(r.sizes, total)
	return r.mock.RunTurn(ctx, req, s)
}

func TestRunCompactsHistory(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	rec := &messageRecorder{mock: &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
	}}}
	svc.Lookup = func(id string) (loop.Provider, error) { return rec, nil }
	header := helpers.CreateRunSession(t, sessions)
	// Seed a history dominated by one 600KB tool result (~150k tokens,
	// over the 128k*0.85 synthetic threshold for the mock model).
	huge, _ := json.Marshal(map[string]any{"role": "toolResult", "results": []any{
		map[string]any{"toolCallId": "c1", "content": strings.Repeat("z", 600_000)},
	}})
	seedUser, _ := json.Marshal(map[string]any{"role": "user", "content": "first"})
	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{
		{ID: "u1", Role: "user", Data: seedUser},
		{ID: "t1", Role: "toolResult", Data: huge},
	}); err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "go", Provider: "mock", ModelID: "mock-model"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	if len(rec.sizes) == 0 {
		t.Fatal("provider never called")
	}
	if rec.sizes[0] > 100_000 {
		t.Fatalf("provider payload not compacted: %d bytes", rec.sizes[0])
	}
}
