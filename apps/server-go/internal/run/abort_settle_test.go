package run

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// wedgedProvider blocks in RunTurn forever without touching ctx or the event
// stream — the shape of a provider stream that ignores cancellation.
type wedgedProvider struct {
	started chan struct{}
}

func (p *wedgedProvider) RunTurn(_ context.Context, _ loop.TurnRequest, _ *stream.Stream[loop.Event]) error {
	select {
	case p.started <- struct{}{}:
	default:
	}
	select {}
}

// instantProvider completes one trivial turn.
type instantProvider struct{}

func (p *instantProvider) RunTurn(_ context.Context, _ loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Push(loop.Event{Kind: loop.EventTurnDone, StopReason: "end_turn"})
	s.Complete()
	return nil
}

// An aborted run that never unwinds must not hold the session's active slot:
// without the post-abort force-settle every later StartRun fails with
// ErrActive until the 15-minute hub watchdog fires.
func TestAbortForceReleasesWedgedRun(t *testing.T) {
	dir := t.TempDir()
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: dir, ModelID: "m", Provider: "fake",
	})
	if err != nil {
		t.Fatal(err)
	}

	wedged := &wedgedProvider{started: make(chan struct{}, 1)}
	svc := NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return wedged, nil }
	svc.AbortSettleTimeout = 50 * time.Millisecond

	if _, err := svc.StartRun(header.ID, Prompt{Text: "hi"}); err != nil {
		t.Fatalf("start run: %v", err)
	}
	select {
	case <-wedged.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run never reached the provider")
	}

	if !svc.Abort(header.ID) {
		t.Fatal("abort reported no active run")
	}

	deadline := time.Now().Add(3 * time.Second)
	for svc.IsActive(header.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if svc.IsActive(header.ID) {
		t.Fatal("wedged run still holds the session slot after abort")
	}

	// The freed slot accepts a new run.
	svc.Lookup = func(string) (loop.Provider, error) { return &instantProvider{}, nil }
	if _, err := svc.StartRun(header.ID, Prompt{Text: "again"}); err != nil {
		t.Fatalf("second start run: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for svc.IsActive(header.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if svc.IsActive(header.ID) {
		t.Fatal("second run did not settle")
	}
}

// A healthy abort settles on its own; the force-settle timer must not fire
// and disturb the session afterwards.
func TestAbortSettleTimerLeavesSettledRunAlone(t *testing.T) {
	dir := t.TempDir()
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: dir, ModelID: "m", Provider: "fake",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Provider that respects ctx: blocks until cancelled, then fails the
	// stream so the run unwinds normally.
	ctxAware := &ctxAwareProvider{ch: make(chan struct{})}
	svc := NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return ctxAware, nil }
	svc.AbortSettleTimeout = 50 * time.Millisecond

	if _, err := svc.StartRun(header.ID, Prompt{Text: "hi"}); err != nil {
		t.Fatalf("start run: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !ctxAware.started() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !svc.Abort(header.ID) {
		t.Fatal("abort reported no active run")
	}
	deadline = time.Now().Add(3 * time.Second)
	for svc.IsActive(header.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if svc.IsActive(header.ID) {
		t.Fatal("ctx-aware run did not settle after abort")
	}
	// Wait past the force-settle window: the slot must stay free and a new
	// run must still start.
	time.Sleep(150 * time.Millisecond)
	svc.Lookup = func(string) (loop.Provider, error) { return &instantProvider{}, nil }
	if _, err := svc.StartRun(header.ID, Prompt{Text: "again"}); err != nil {
		t.Fatalf("start after settle window: %v", err)
	}
}

type ctxAwareProvider struct {
	ch   chan struct{}
	once sync.Once
}

func (p *ctxAwareProvider) started() bool {
	select {
	case <-p.ch:
		return true
	default:
		return false
	}
}

func (p *ctxAwareProvider) RunTurn(ctx context.Context, _ loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	p.once.Do(func() { close(p.ch) })
	<-ctx.Done()
	s.Fail(ctx.Err())
	return ctx.Err()
}
