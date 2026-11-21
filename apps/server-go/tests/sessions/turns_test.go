package tests

import (
	"fmt"
	"strings"
	"testing"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func userMsg(id, text string) types.AgentMessage {
	return types.AgentMessage{ID: id, Role: "user", Data: []byte(`{"user":{"content":"` + text + `"}}`)}
}

func wakeMsg(id, label string) types.AgentMessage {
	return types.AgentMessage{ID: id, Role: "user", Data: []byte(`{"user":{"content":"x","synthetic":true,"wakeLabel":"` + label + `"}}`)}
}

func callMsg(id, callID string) types.AgentMessage {
	return types.AgentMessage{ID: id, Role: "assistant", Data: []byte(`{"assistant":{"id":"` + id + `","content":[{"toolCall":{"id":"` + callID + `","name":"read_file","arguments":"` + enc(`{}`) + `"}}],"stopReason":"toolUse"}}`)}
}

func resultMsg(id, callID, body string) types.AgentMessage {
	return types.AgentMessage{ID: id, Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"` + callID + `","toolName":"read_file","content":"` + enc(body) + `"}]}}`)}
}

func replyMsg(id, text string) types.AgentMessage {
	return types.AgentMessage{ID: id, Role: "assistant", Data: []byte(`{"assistant":{"id":"` + id + `","content":[{"text":{"text":"` + text + `"}}],"stopReason":"endTurn"}}`)}
}

func manyToolCalls(prefix string, n int) []types.AgentMessage {
	var out []types.AgentMessage
	for i := 0; i < n; i++ {
		c := fmt.Sprintf("%s-c%d", prefix, i)
		out = append(out, callMsg(c+"-a", c), resultMsg(c+"-r", c, "ok"))
	}
	return out
}

func TestTurnPageNeverSplitsATurn(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	var msgs []types.AgentMessage
	msgs = append(msgs, userMsg("u1", "first"))
	msgs = append(msgs, manyToolCalls("t1", 30)...)
	msgs = append(msgs, replyMsg("a1", "done one"))
	msgs = append(msgs, userMsg("u2", "second"), replyMsg("a2", "done two"))
	if err := s.AppendMessages(h.ID, msgs); err != nil {
		t.Fatal(err)
	}

	page, err := s.LoadTurns(h.ID, 1, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 1 || page.Turns[0].GetId() != "u2" || !page.HasMore || page.NextCursor == nil {
		t.Fatalf("newest page = %d turns, has_more=%v, cursor=%v", len(page.Turns), page.HasMore, page.NextCursor)
	}

	older, err := s.LoadTurns(h.ID, 1, *page.NextCursor, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Turns) != 1 || older.HasMore {
		t.Fatalf("older page = %d turns, has_more=%v", len(older.Turns), older.HasMore)
	}
	turn := older.Turns[0]
	if turn.GetId() != "u1" || len(turn.GetRun().GetEvents()) != 30 || len(turn.GetResponses()) != 1 {
		t.Fatalf("turn u1 events=%d responses=%d", len(turn.GetRun().GetEvents()), len(turn.GetResponses()))
	}
	for _, e := range turn.GetRun().GetEvents() {
		if e.GetToolCall().GetResult() == nil {
			t.Fatalf("call %s has no paired result", e.GetId())
		}
	}
}

func TestTurnSplitsOnWakeRow(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	msgs := []types.AgentMessage{
		userMsg("u1", "go"), callMsg("a1", "c1"), resultMsg("r1", "c1", "ok"), replyMsg("a2", "started"),
		wakeMsg("w1", "Bash finished"), callMsg("a3", "c2"), resultMsg("r2", "c2", "ok"), replyMsg("a4", "all done"),
	}
	if err := s.AppendMessages(h.ID, msgs); err != nil {
		t.Fatal(err)
	}
	page, err := s.LoadTurns(h.ID, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(page.Turns))
	}
	wake := page.Turns[1]
	if wake.GetWakeLabel() != "Bash finished" || wake.UserMessage != nil || len(wake.GetRun().GetEvents()) != 1 {
		t.Fatalf("wake turn = %+v", wake)
	}
}

func TestOpenTurnIsWorkingAndMissingResultStaysUnresolved(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{userMsg("u1", "go"), callMsg("a1", "c1")}); err != nil {
		t.Fatal(err)
	}
	page, err := s.LoadTurns(h.ID, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	run := page.Turns[0].GetRun()
	if run.GetStatus() != consolev1.RunStatus_RUN_STATUS_WORKING {
		t.Fatalf("status = %v", run.GetStatus())
	}
	if run.GetEvents()[0].GetToolCall().Result != nil {
		t.Fatal("unpaired call must have no result")
	}
}

func TestLargeResultIsOmittedAndFetchedOnDemand(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	big := strings.Repeat("x", 10000)
	if err := s.AppendMessages(h.ID, []types.AgentMessage{
		userMsg("u1", "go"), callMsg("a1", "c1"), resultMsg("r1", "c1", big), replyMsg("a2", "ok"),
	}); err != nil {
		t.Fatal(err)
	}
	page, err := s.LoadTurns(h.ID, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	call := page.Turns[0].GetRun().GetEvents()[0].GetToolCall()
	if len(call.GetResult().GetContent()) != 0 || call.GetResultSize() != 10000 || call.GetResultMessageId() != "r1" {
		t.Fatalf("summary content=%d size=%d msg=%q", len(call.GetResult().GetContent()), call.GetResultSize(), call.GetResultMessageId())
	}
	full, err := s.ToolResult(h.ID, call.GetResultMessageId(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if string(full.GetContent()) != big {
		t.Fatalf("full result length = %d", len(full.GetContent()))
	}
}
