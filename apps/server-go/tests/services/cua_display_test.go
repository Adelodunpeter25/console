// Display resolution: an existing display wins, a live socket is attached when
// DISPLAY is missing or stale, and the failure names the problem.
package tests

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func listenX(t *testing.T, dir string, n string) {
	t.Helper()
	l, err := net.Listen("unix", filepath.Join(dir, "X"+n))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "x11")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestDisplayKeepsReachableDisplay(t *testing.T) {
	dir := shortDir(t)
	listenX(t, dir, "5")
	listenX(t, dir, "1")
	got, err := cua.ResolveDisplayForTest(":5", dir, os.Getuid())
	if err != nil || got != ":5" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDisplayAttachesToLiveSocketWhenUnset(t *testing.T) {
	dir := shortDir(t)
	listenX(t, dir, "7")
	listenX(t, dir, "3")
	got, err := cua.ResolveDisplayForTest("", dir, os.Getuid())
	if err != nil || got != ":3" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDisplayReplacesStaleDisplay(t *testing.T) {
	dir := shortDir(t)
	listenX(t, dir, "2")
	got, err := cua.ResolveDisplayForTest(":99", dir, os.Getuid())
	if err != nil || got != ":2" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDisplayIgnoresDeadSocketAndReportsNone(t *testing.T) {
	dir := shortDir(t)
	if err := os.WriteFile(filepath.Join(dir, "X4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := cua.ResolveDisplayForTest("", dir, os.Getuid())
	if !errors.Is(err, cua.ErrNoDisplay) {
		t.Fatalf("want ErrNoDisplay, got %v", err)
	}
}

func TestDisplayIgnoresSocketsOwnedByOthers(t *testing.T) {
	dir := shortDir(t)
	listenX(t, dir, "6")
	_, err := cua.ResolveDisplayForTest("", dir, os.Getuid()+1)
	if !errors.Is(err, cua.ErrNoDisplay) {
		t.Fatalf("want ErrNoDisplay, got %v", err)
	}
}
