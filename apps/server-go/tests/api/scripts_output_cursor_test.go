// Cursor pagination over a script run's output: absolute offsets that stay
// valid after the 256KB buffer trims its head.
package tests

import (
	"strings"
	"testing"
	"time"
)

func waitScriptDone(t *testing.T, scriptsRunning func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for scriptsRunning() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if scriptsRunning() {
		t.Fatal("script did not finish")
	}
}

func TestReadOutputCursorRoundTrip(t *testing.T) {
	scripts, projectID, root := newScriptService(t)
	writeConsoleToml(t, root, `
[scripts.say]
label = "Say"
command = "printf 'alpha-' && printf 'beta-' && printf 'gamma'"
`)
	run, err := scripts.Run(projectID, "say")
	if err != nil {
		t.Fatal(err)
	}
	waitScriptDone(t, func() bool { return scripts.IsRunning(projectID, run.RunID) })

	first, err := scripts.ReadOutput(projectID, run.RunID, "stdout", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != "alpha-" || first.Next != 6 || first.Truncated {
		t.Fatalf("first page = %+v", first)
	}
	// Feeding Next back continues exactly where the last page stopped.
	rest, err := scripts.ReadOutput(projectID, run.RunID, "stdout", first.Next, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rest.Text != "beta-gamma" || rest.Next != 16 {
		t.Fatalf("rest = %+v", rest)
	}
	// Caught up: nothing new, and the cursor does not move.
	done, _ := scripts.ReadOutput(projectID, run.RunID, "stdout", rest.Next, 0)
	if done.Text != "" || done.Next != rest.Next {
		t.Fatalf("caught-up read = %+v", done)
	}
	// stderr is a separate stream with its own offsets.
	errOut, _ := scripts.ReadOutput(projectID, run.RunID, "stderr", 0, 0)
	if errOut.Text != "" || errOut.Next != 0 {
		t.Fatalf("stderr = %+v", errOut)
	}
}

func TestReadOutputCursorSurvivesTrimming(t *testing.T) {
	scripts, projectID, root := newScriptService(t)
	// 400KB of a repeating marker: well past the 256KB retained buffer.
	writeConsoleToml(t, root, `
[scripts.flood]
label = "Flood"
command = "i=0; while [ $i -lt 400 ]; do head -c 1024 /dev/zero | tr '\\0' 'x'; i=$((i+1)); done; printf END"
`)
	run, err := scripts.Run(projectID, "flood")
	if err != nil {
		t.Fatal(err)
	}
	waitScriptDone(t, func() bool { return scripts.IsRunning(projectID, run.RunID) })

	const written = 400*1024 + 3
	// A cursor at 0 fell off the head: it resumes at the oldest kept byte and says so.
	chunk, err := scripts.ReadOutput(projectID, run.RunID, "stdout", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !chunk.Truncated {
		t.Fatalf("expected Truncated, got %+v", chunk)
	}
	if chunk.From != written-256*1024 {
		t.Fatalf("resumed at %d, want %d", chunk.From, written-256*1024)
	}
	if chunk.Next != written {
		t.Fatalf("next = %d, want the absolute total %d", chunk.Next, written)
	}
	if len(chunk.Text) != 256*1024 || !strings.HasSuffix(chunk.Text, "END") {
		t.Fatalf("kept tail wrong: len=%d", len(chunk.Text))
	}
	// A cursor still inside the retained window is not truncated.
	inside, _ := scripts.ReadOutput(projectID, run.RunID, "stdout", written-10, 0)
	if inside.Truncated || len(inside.Text) != 10 || !strings.HasSuffix(inside.Text, "END") {
		t.Fatalf("inside read = %+v", inside)
	}
}

func TestReadOutputRejectsBadInput(t *testing.T) {
	scripts, projectID, _ := newScriptService(t)
	if _, err := scripts.ReadOutput(projectID, "nope", "stdout", 0, 0); err == nil {
		t.Fatal("unknown run must fail")
	}
	if _, err := scripts.ReadOutput(projectID, "nope", "stdin", 0, 0); err == nil {
		t.Fatal("unknown run must fail before the stream is considered")
	}
}
