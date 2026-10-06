// Context-window occupancy: estimate math, message-count cache reuse, and
// unknown-session mapping.
package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestContextUsageEstimateAndCache(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	header := helpers.CreateRunSession(t, sessions)

	seedUser, err := loop.ToProtoBytes(loop.UserMessage{Role: loop.RoleUser, Content: "hello there"})
	if err != nil {
		t.Fatal(err)
	}
	seedAssistant, err := loop.ToProtoBytes(loop.AssistantMessage{
		Role: loop.RoleAssistant, ID: "m1", StopReason: loop.StopStop,
		Content: []any{loop.TextPart{Type: "text", Text: "hi back"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{
		{ID: "u1", Role: "user", Data: seedUser},
		{ID: "m1", Role: "assistant", Data: seedAssistant},
	}); err != nil {
		t.Fatal(err)
	}

	first, err := svc.ContextUsage(context.Background(), header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.UsedTokens <= 0 || first.ContextWindow <= 0 {
		t.Fatalf("empty snapshot: %+v", first)
	}
	if first.PercentUsed <= 0 || first.Source == "" || first.Provider == "" {
		t.Fatalf("snapshot missing fields: %+v", first)
	}

	// Same message count: cached copy, identical values.
	second, err := svc.ContextUsage(context.Background(), header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("cache miss: %+v vs %+v", second, first)
	}

	// New message: recompute, tokens grow.
	more, err := loop.ToProtoBytes(loop.UserMessage{
		Role:    loop.RoleUser,
		Content: "and another fairly long message here",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.AppendMessage(header.ID, types.AgentMessage{ID: "u2", Role: "user", Data: more}); err != nil {
		t.Fatal(err)
	}
	third, err := svc.ContextUsage(context.Background(), header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if third.UsedTokens <= first.UsedTokens {
		t.Fatalf("stale after growth: %+v vs %+v", third, first)
	}
}

func TestContextUsageUnknownSession(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	if _, err := svc.ContextUsage(context.Background(), "does-not-exist"); !errors.Is(err, run.ErrNoSession) {
		t.Fatalf("err = %v; want ErrNoSession", err)
	}
}
