// Devin marks the last stable message as a cache breakpoint; the per-request
// tail sits after it so it never enters the cached prefix.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/devin"
)

func marked(p *exav1.ChatMessagePrompt) bool {
	return p.GetPromptCacheOptions().GetType() == exav1.CacheControlType_CACHE_CONTROL_TYPE_EPHEMERAL
}

func TestDevinMarksLastStableMessage(t *testing.T) {
	history := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: "one"},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{loop.TextPart{Type: "text", Text: "two"}}},
		loop.UserMessage{Role: loop.RoleUser, Content: "three"},
	}
	prompts := devin.BuildPromptsWithCache(history, "conv", "", loop.CacheShort)
	if len(prompts) != 3 || !marked(prompts[2]) || marked(prompts[0]) || marked(prompts[1]) {
		t.Fatalf("only the last message should carry the breakpoint: %v", prompts)
	}
}

func TestDevinTailStaysAfterTheBreakpoint(t *testing.T) {
	history := []any{loop.UserMessage{Role: loop.RoleUser, Content: "one"}}
	prompts := devin.BuildPromptsWithCache(history, "conv", "notice", loop.CacheShort)
	if len(prompts) != 2 || !marked(prompts[0]) || marked(prompts[1]) {
		t.Fatalf("the breakpoint must sit before the tail: %v", prompts)
	}
}

func TestDevinNoMarkerWhenCachingIsOff(t *testing.T) {
	history := []any{loop.UserMessage{Role: loop.RoleUser, Content: "one"}}
	for _, p := range devin.BuildPromptsWithCache(history, "conv", "", loop.CacheNone) {
		if marked(p) {
			t.Fatal("retention none must not mark anything")
		}
	}
}
