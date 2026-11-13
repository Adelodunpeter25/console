// Coverage for tool-output spilling: large output is saved to a file and the
// useful side (tail for command output, head for fetches) is shown.
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

func spillPathAfter(notice, marker string) string {
	i := strings.Index(notice, marker)
	if i < 0 {
		return ""
	}
	rest := notice[i+len(marker):]
	end := strings.IndexAny(rest, "\n")
	if end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), " ...]"))
}

func TestBashSpillsLargeOutputAndKeepsTheTail(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command": "for i in $(seq 1 5000); do echo \"progress line $i\"; done; echo 'panic: the actual error'",
	})
	out, err := tools.Bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "full output saved to ") {
		t.Fatalf("missing spill notice: %q", s)
	}
	if !strings.Contains(s, "panic: the actual error") {
		t.Fatalf("tail lost the error: %q", s)
	}
	if strings.Contains(s, "progress line 100") {
		t.Fatalf("head should not be shown: %q", s)
	}
	path := spillPathAfter(s, "full output saved to ")
	if path == "" {
		t.Fatalf("no spill path: %q", s)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if !strings.Contains(string(data), "panic: the actual error") || !strings.Contains(string(data), "progress line 1\n") {
		t.Fatal("spill file does not hold the full output")
	}
}

func TestBashSmallOutputUnchanged(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "echo hello"})
	out, err := tools.Bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "hello") || strings.Contains(s, "full output saved to") {
		t.Fatalf("small output changed: %q", s)
	}
}

func TestWebFetchSpillsLargeBody(t *testing.T) {
	end := "\nUNIQUE-END-MARKER"
	body := strings.Repeat("x", 600*1024) + end
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"url": srv.URL + "/api/big"})
	out, err := tools.Fetch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Full body saved to ") {
		t.Fatalf("missing spill notice: %q", s)
	}
	path := spillPathAfter(s, "Full body saved to ")
	if path == "" {
		t.Fatalf("no spill path: %q", s)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if !strings.HasSuffix(string(data), end) {
		t.Fatal("spill file does not hold the full body")
	}
}

func TestBashJobSpillsRingBufferDrops(t *testing.T) {
	jobs := terminal.NewBashJobManager(nil)
	snap, err := jobs.Start(terminal.StartBashJobOptions{
		Command:  "for i in $(seq 1 40000); do echo \"line $i\"; done; echo JOB-DONE",
		TimeoutMs: 60_000,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer jobs.KillOwnedBy("")

	// Poll until the ring buffer has dropped something (256 KB window).
	var out terminal.BashJobOutput
	for i := 0; i < 100; i++ {
		out, err = jobs.Output(snap.JobID, "", 0, 50_000)
		if err != nil {
			t.Fatalf("output: %v", err)
		}
		if out.StdoutDropped > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if out.StdoutDropped == 0 {
		t.Fatal("ring buffer never dropped output")
	}
	if out.StdoutSpill == "" {
		t.Fatalf("no spill path for dropped output: %+v", out)
	}
	data, err := os.ReadFile(out.StdoutSpill)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	// The spill file reconstructs the dropped prefix, in order.
	if !strings.Contains(string(data), "line 1\n") {
		t.Fatal("spill file missing the first dropped line")
	}
	if len(data) != out.StdoutDropped {
		t.Fatalf("spill file holds %d bytes, dropped counter says %d", len(data), out.StdoutDropped)
	}
}
