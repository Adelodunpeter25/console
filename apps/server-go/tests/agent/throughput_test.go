// Per-turn output throughput: tokens over provider-active stream time, with
// reasoning folded in where the provider reports it separately.
package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestTurnThroughputPersistsOnAssistantMessage(t *testing.T) {
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			// Long enough for the stream-time floor: an instantaneous mock
			// would be correctly suppressed as unmeasurable.
			time.Sleep(300 * time.Millisecond)
			return []loop.Event{
				{Kind: loop.EventText, Text: "hi"},
				usageEvent(100, 0, 0, 40),
			}
		},
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	events, err := agent.Run(context.Background(), "s1", "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}

	var turn *loop.AssistantMessage
	for {
		event, err, ok := events.Next()
		if !ok {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.Kind == loop.EventModelStreamEnd && event.Message != nil {
			if msg, ok := event.Message.(loop.AssistantMessage); ok {
				turn = &msg
			}
		}
	}
	if turn == nil {
		t.Fatal("no modelStreamEnd turn")
	}
	if turn.TokPerSec == nil {
		t.Fatal("tokPerSec missing from the turn")
	}
	if *turn.TokPerSec <= 0 {
		t.Fatalf("tokPerSec = %v, want > 0", *turn.TokPerSec)
	}

	// The same value must survive the storage round trip, so history keeps
	// its throughput after reload.
	raw, err := loop.ToProtoBytes(*turn)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	assistant, ok := restored.(loop.AssistantMessage)
	if !ok {
		t.Fatalf("restored %T, want loop.AssistantMessage", restored)
	}
	if assistant.TokPerSec == nil || *assistant.TokPerSec != *turn.TokPerSec {
		t.Fatalf("tokPerSec round trip = %v, want %v", assistant.TokPerSec, *turn.TokPerSec)
	}

	// The desktop reads the camelCase JSON key off the wire frame, so the
	// field has to survive protojson, not just the struct round trip.
	frame, err := protojson.Marshal(loop.AssistantToProto(*turn))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(frame), `"tokPerSec"`) {
		t.Fatalf("wire frame %s missing tokPerSec", frame)
	}
}

func TestTurnThroughputOmittedWithoutUsage(t *testing.T) {
	// A provider that reports no usage block leaves the turn unmeasurable;
	// the field must stay absent rather than claiming zero throughput.
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: "hi"}}
		},
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	events, err := agent.Run(context.Background(), "s1", "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	for {
		event, _, ok := events.Next()
		if !ok {
			break
		}
		if event.Kind == loop.EventModelStreamEnd && event.Message != nil {
			msg, ok := event.Message.(loop.AssistantMessage)
			if !ok {
				t.Fatalf("turn payload %T, want loop.AssistantMessage", event.Message)
			}
			if msg.TokPerSec != nil {
				t.Fatalf("tokPerSec = %v, want absent", *msg.TokPerSec)
			}
			return
		}
	}
	t.Fatal("no modelStreamEnd turn")
}
