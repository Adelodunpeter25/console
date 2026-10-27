// Vision handoff: the describer makes exactly one tool-free, attachment-
// carrying call and returns its trimmed prose.
package tests

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/vision"
)

type captureProvider struct {
	req    loop.TurnRequest
	events []loop.Event
}

func (c *captureProvider) RunTurn(_ context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	c.req = req
	for _, e := range c.events {
		s.Push(e)
	}
	s.Complete()
	return nil
}

func TestVisionDescribe(t *testing.T) {
	p := &captureProvider{events: []loop.Event{
		{Kind: loop.EventThinking, Text: "ignore me"},
		{Kind: loop.EventText, Text: "  A blue square.\n"},
	}}
	got := vision.Describe(context.Background(), p, "vision-model", "image/png", []byte("raw"), "/a/b.png", "")
	if got != "A blue square." {
		t.Fatalf("describe: %q", got)
	}
	if p.req.Model != "vision-model" || len(p.req.Tools) != 0 {
		t.Fatalf("request must be tool-free on the vision model: %+v", p.req)
	}
	if len(p.req.Messages) != 1 {
		t.Fatalf("messages: %d", len(p.req.Messages))
	}
	msg, ok := p.req.Messages[0].(loop.UserMessage)
	if !ok || len(msg.Attachments) != 1 {
		t.Fatalf("message: %#v", p.req.Messages[0])
	}
	want := base64.StdEncoding.EncodeToString([]byte("raw"))
	if msg.Attachments[0].MimeType != "image/png" || msg.Attachments[0].Data != want {
		t.Fatalf("attachment: %#v", msg.Attachments[0])
	}
}
