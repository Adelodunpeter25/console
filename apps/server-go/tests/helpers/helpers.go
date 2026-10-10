// Shared test doubles and fixtures.
// One home for the mocks every suite reuses so grouped test packages stay
// decoupled from each other.
package helpers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// MockProvider replays scripted turns; used to drive loops and runs
// without a real model backend.
type MockProvider struct {
	Turns []func() []loop.Event
	calls int
}

func (m *MockProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	script := m.Turns[min(m.calls, len(m.Turns)-1)]
	m.calls++
	for _, event := range script() {
		s.Push(event)
	}
	s.Complete()
	return nil
}

// AutoApprover approves every permission request.
type AutoApprover struct{}

func (AutoApprover) Approve(ctx context.Context, req permissions.Request) (bool, error) {
	return true, nil
}

// MustJSONRaw marshals a value or fails the test.
func MustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// NewRunSessions opens an isolated in-memory session service.
func NewRunSessions(t *testing.T) *services.SessionService {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return services.NewSessionService(manager)
}

// CreateRunSession creates a scratch session for run tests.
// TempDir returns a fresh temp directory with symlinks resolved. The path
// guard hands tools the symlink-resolved path it verified, so tests that
// compare a recorded path against the one they asked for must build it from a
// resolved directory (on macOS t.TempDir returns /var/... while the real path
// is /private/var/...).
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return dir
}

// CreateRunSession creates a run session in a fresh temp directory.
func CreateRunSession(t *testing.T, sessions *services.SessionService) types.SessionHeader {
	t.Helper()
	return CreateRunSessionIn(t, sessions, t.TempDir())
}

// CreateRunSessionIn creates a run session whose cwd is dir. Runs confine
// writes to the session root, so a test that writes files must create the
// session in the same directory it writes to.
func CreateRunSessionIn(t *testing.T, sessions *services.SessionService, dir string) types.SessionHeader {
	t.Helper()
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: dir, ModelID: "mock-model", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}
	return header
}

// WaitSettled waits for a run hub to close or fails the test.
func WaitSettled(t *testing.T, hub *run.Hub) {
	t.Helper()
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}
}
