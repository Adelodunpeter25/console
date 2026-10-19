// Full-loop check that an image read by a tool reaches the model on the turn
// that acts on it, even though the payload is never persisted. The model must
// see the picture in the same run; only the stored copy is stripped.
package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// imageProvider records every request it receives and replays a scripted
// sequence: one turn that calls a tool, then a final text turn.
type imageProvider struct {
	requests []loop.TurnRequest
	calls    int
}

func (p *imageProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	p.requests = append(p.requests, req)
	switch p.calls {
	case 0:
		p.calls++
		s.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
			ID:        "c1",
			Name:      "shot",
			Arguments: json.RawMessage(`{}`),
		}})
	default:
		p.calls++
		s.Push(loop.Event{Kind: loop.EventText, Text: "done"})
	}
	s.Complete()
	return nil
}

// shotTool returns a tool result carrying an inline image, the way read_file
// does for a real picture.
func shotTool() tools.Tool {
	return tools.NewTool("shot", "returns an image", tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) {
			return []map[string]any{
				{"type": "text", "text": "Image read: /tmp/shot.png"},
				{"type": "image", "data": "THE-IMAGE-BYTES", "mimeType": "image/png"},
			}, nil
		})
}

// The follow-up provider request (the one that acts on the tool result) must
// still carry the image, even though the stored copy does not.
func TestModelSeesImageOnActingTurn(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	provider := &imageProvider{}

	registry := tools.NewRegistry(shotTool())
	executor := loop.NewExecutor(registry, permissions.FullAccess, helpers.AutoApprover{})
	agent := loop.New(provider, executor, sessions)

	events, err := agent.Run(context.Background(), "sess-1", "look at the image", nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, _, ok := events.Next(); !ok {
			break
		}
	}

	if len(provider.requests) < 2 {
		t.Fatalf("expected two provider requests, got %d", len(provider.requests))
	}

	// The second request carries the tool result. The image must be present.
	second := provider.requests[1]
	found := false
	for _, msg := range second.Messages {
		results, ok := msg.(loop.ToolResultMessage)
		if !ok {
			continue
		}
		for _, r := range results.Results {
			parts, ok := r.Content.([]map[string]any)
			if !ok {
				continue
			}
			for _, p := range parts {
				if p["type"] == "image" && p["data"] == "THE-IMAGE-BYTES" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("the model must receive the real image on the turn that acts on it")
	}
}

// The same run's stored copy must carry no image bytes.
func TestStoredCopyAfterImageRunHasNoBytes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	provider := &imageProvider{}
	header := helpers.CreateRunSession(t, sessions)

	registry := tools.NewRegistry(shotTool())
	executor := loop.NewExecutor(registry, permissions.FullAccess, helpers.AutoApprover{})
	agent := loop.New(provider, executor, sessions)

	events, err := agent.Run(context.Background(), header.ID, "look at the image", nil)
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, _, ok := events.Next(); !ok {
			break
		}
	}

	// The image bytes must not appear anywhere in the persisted session. The
	// tool-result content is stored as JSON inside a proto bytes field, so the
	// check decodes the stored message rather than searching raw bytes.
	stored := storedMessages(t, sessions, header.ID)
	if len(stored) == 0 {
		t.Fatal("expected stored messages")
	}
	for _, raw := range stored {
		decoded, err := loop.MessageFromProtoBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		results, ok := decoded.(loop.ToolResultMessage)
		if !ok {
			continue
		}
		for _, r := range results.Results {
			parts, ok := r.Content.([]any)
			if !ok {
				continue
			}
			for _, part := range parts {
				m, _ := part.(map[string]any)
				if m != nil && m["type"] == "image" {
					t.Fatalf("stored message still carries an image part: %#v", m)
				}
			}
		}
	}
}

// storedMessages returns every stored message row for a session.
func storedMessages(t *testing.T, sessions *services.SessionService, sessionID string) [][]byte {
	t.Helper()
	loaded, err := sessions.Load(sessionID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		return nil
	}
	out := make([][]byte, 0, len(loaded.Messages))
	for _, m := range loaded.Messages {
		out = append(out, m)
	}
	return out
}
