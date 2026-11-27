// A dropped terminal socket must not kill the shell: output produced while
// detached is delivered to the next socket that attaches.
package tests

import (
	"strings"
	"sync"
	"testing"
	"time"

	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type outputSink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (o *outputSink) write(b []byte) {
	o.mu.Lock()
	o.buf.Write(b)
	o.mu.Unlock()
}

func (o *outputSink) contains(s string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return strings.Contains(o.buf.String(), s)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestTerminalReattachDeliversOutputFromDetachedPeriod(t *testing.T) {
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()
	session, err := m.Spawn(types.TerminalSpawnParams{Cwd: t.TempDir(), Shell: "/bin/sh", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}

	first := &outputSink{}
	gen1 := session.Attach(first.write, func(int) {})
	session.Detach(gen1)

	// Printed while no socket is attached; the typed command text itself
	// never contains the joined marker, only the shell's output does.
	_ = session.Write([]byte("echo held-$((20+22))\n"))
	time.Sleep(200 * time.Millisecond)
	if first.contains("held-42") {
		t.Fatal("detached output leaked to the old socket")
	}

	second := &outputSink{}
	session.Attach(second.write, func(int) {})
	waitFor(t, func() bool { return second.contains("held-42") })

	// The old socket's late Detach must not detach the new one.
	session.Detach(gen1)
	_ = session.Write([]byte("echo live-$((1+2))\n"))
	waitFor(t, func() bool { return second.contains("live-3") })

	if m.Get(session.ID) == nil {
		t.Fatal("session was removed")
	}
}
