package tests

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// TestDirectReadParityWithCanonicalLoad verifies that reading history directly
// via LoadSplit + MessageFromSplit matches the canonical Load + DecodeHistory path
// exactly for all message types.
func TestDirectReadParityWithCanonicalLoad(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)

	testMessages := []types.AgentMessage{
		{
			ID:   "u1",
			Role: "user",
			Data: []byte(`{"user":{"content":"hello world","contextFiles":["main.go"]}}`),
		},
		{
			ID:   "a1",
			Role: "assistant",
			Data: []byte(`{"assistant":{"id":"a1","content":[{"text":{"text":"I will check the file."}},{"thinking":{"text":"Let us inspect auth."}},{"toolCall":{"id":"tc1","name":"read_file","arguments":"` + enc(`{"path":"auth.go"}`) + `"}}],"stopReason":"toolUse"}}`),
		},
		{
			ID:   "r1",
			Role: "toolResult",
			Data: []byte(`{"toolResult":{"results":[{"toolCallId":"tc1","toolName":"read_file","content":"` + enc(`{"status":"ok","lines":42}`) + `","isError":false}]}}`),
		},
		{
			ID:   "a2",
			Role: "assistant",
			Data: []byte(`{"assistant":{"id":"a2","content":[{"text":{"text":"All looks good."}}],"stopReason":"endTurn"}}`),
		},
		{
			ID:   "r2",
			Role: "toolResult",
			Data: []byte(`{"toolResult":{"results":[{"toolCallId":"tc2","toolName":"bash","content":"` + enc(`"plain string output"`) + `","isError":true}]}}`),
		},
	}

	if err := s.AppendMessages(h.ID, testMessages); err != nil {
		t.Fatalf("append: %v", err)
	}

	// Canonical path: Load -> Assemble -> withCreatedAt -> protojson.Unmarshal -> loop struct
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil {
		t.Fatalf("load canonical: %v", err)
	}
	canonicalHistory := run.DecodeHistory(loaded.Messages)

	// Direct path: LoadSplit -> MessageFromSplit
	runSvc := run.NewService(s)
	directHistory, err := runSvc.LoadHistory(h.ID)
	if err != nil {
		t.Fatalf("load direct: %v", err)
	}

	if len(directHistory) != len(canonicalHistory) {
		t.Fatalf("count mismatch: direct=%d, canonical=%d", len(directHistory), len(canonicalHistory))
	}

	for i := range canonicalHistory {
		cRaw, err := json.Marshal(canonicalHistory[i])
		if err != nil {
			t.Fatalf("marshal canonical %d: %v", i, err)
		}
		dRaw, err := json.Marshal(directHistory[i])
		if err != nil {
			t.Fatalf("marshal direct %d: %v", i, err)
		}
		var cMap, dMap map[string]any
		_ = json.Unmarshal(cRaw, &cMap)
		_ = json.Unmarshal(dRaw, &dMap)
		if !reflect.DeepEqual(cMap, dMap) {
			t.Errorf("message %d differs:\ncanonical: %s\ndirect:    %s", i, cRaw, dRaw)
		}
	}
}

// TestHeaderOnlyReadBypassesMessageRebuild ensures Header() works on sessions
// without loading messages table or assembling parts.
func TestHeaderOnlyReadBypassesMessageRebuild(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)

	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}

	header, err := s.Header(h.ID)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if header == nil || header.ID != h.ID || header.Title != h.Title {
		t.Fatalf("unexpected header: %+v", header)
	}
}

func BenchmarkDirectVsCanonicalLoad(b *testing.B) {
	s, _ := partsStore(&testing.T{})
	h, err := s.Create(types.CreateSessionOptions{Title: "t", ModelID: "m", Provider: "p"})
	if err != nil {
		b.Fatal(err)
	}
	var msgs []types.AgentMessage
	for i := 0; i < 50; i++ {
		msgs = append(msgs,
			types.AgentMessage{ID: fmt.Sprintf("u%d", i), Role: "user", Data: []byte(`{"user":{"content":"do something"}}`)},
			types.AgentMessage{ID: fmt.Sprintf("a%d", i), Role: "assistant", Data: []byte(`{"assistant":{"id":"` + fmt.Sprintf("a%d", i) + `","content":[{"toolCall":{"id":"` + fmt.Sprintf("c%d", i) + `","name":"read","arguments":"` + enc(`{"path":"file.go"}`) + `"}}],"stopReason":"toolUse"}}`)},
			types.AgentMessage{ID: fmt.Sprintf("r%d", i), Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"` + fmt.Sprintf("c%d", i) + `","content":"` + enc(`{"lines":100,"code":"package main"}`) + `"}]}}`)},
		)
	}
	if err := s.AppendMessages(h.ID, msgs); err != nil {
		b.Fatal(err)
	}
	runSvc := run.NewService(s)

	b.Run("Canonical_Load_And_DecodeHistory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			loaded, err := s.Load(h.ID, 0, 0)
			if err != nil {
				b.Fatal(err)
			}
			history := run.DecodeHistory(loaded.Messages)
			if len(history) != 150 {
				b.Fatalf("expected 150, got %d", len(history))
			}
		}
	})

	b.Run("Direct_LoadHistory", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			history, err := runSvc.LoadHistory(h.ID)
			if err != nil {
				b.Fatal(err)
			}
			if len(history) != 150 {
				b.Fatalf("expected 150, got %d", len(history))
			}
		}
	})
}
