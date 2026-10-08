// FsWatchService: nested events reach the project, versions are per project,
// and an unsubscribed project releases its directory watches.
package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func newWatcher(t *testing.T) *services.FsWatchService {
	t.Helper()
	w, err := services.NewFsWatchService()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	return w
}

func TestFsWatchNestedEventsReachProject(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t)
	w.Watch(root)
	ch := w.Subscribe(root)
	for i := 0; i < 10; i++ {
		_ = os.WriteFile(filepath.Join(deep, "f.txt"), []byte{byte(i)}, 0o644)
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("event %d in a nested dir never reached the project subscriber", i)
		}
	}
}

func TestFsWatchVersionIsPerProject(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	w := newWatcher(t)
	w.Watch(a)
	w.Watch(b)
	chA := w.Subscribe(a)
	vb := w.Version(b)
	_ = os.WriteFile(filepath.Join(a, "x"), []byte("1"), 0o644)
	select {
	case <-chA:
	case <-time.After(2 * time.Second):
		t.Fatal("no event for project a")
	}
	if w.Version(a) == 0 {
		t.Fatal("project a version did not advance")
	}
	if w.Version(b) != vb {
		t.Fatal("a change in project a must not invalidate project b")
	}
}

func TestFsWatchReleasesIdleProject(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "x", "y"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(t)
	w.SetIdleRelease(100 * time.Millisecond)
	w.Watch(root)
	if w.WatchedDirCount() < 3 {
		t.Fatalf("expected root and subdirs watched, got %d", w.WatchedDirCount())
	}
	ch := w.Subscribe(root)
	time.Sleep(300 * time.Millisecond)
	if w.WatchedDirCount() == 0 {
		t.Fatal("a subscribed project must keep its watches")
	}
	w.Unsubscribe(ch)
	deadline := time.Now().Add(2 * time.Second)
	for w.WatchedDirCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("watches not released after last unsubscribe: %d left", w.WatchedDirCount())
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Watching again after a release works and sees new changes.
	w.Watch(root)
	ch = w.Subscribe(root)
	_ = os.WriteFile(filepath.Join(root, "x", "y", "n.txt"), []byte("1"), 0o644)
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("re-watched project produced no event")
	}
}

func TestFsWatchResubscribeCancelsRelease(t *testing.T) {
	root := t.TempDir()
	w := newWatcher(t)
	w.SetIdleRelease(150 * time.Millisecond)
	w.Watch(root)
	ch := w.Subscribe(root)
	w.Unsubscribe(ch)
	time.Sleep(50 * time.Millisecond)
	_ = w.Subscribe(root)
	time.Sleep(400 * time.Millisecond)
	if w.WatchedDirCount() == 0 {
		t.Fatal("resubscribing must cancel the pending release")
	}
}
