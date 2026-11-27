// Display resolution: an existing display wins, a live socket is attached when
// DISPLAY is missing or stale, and the failure names the problem.
package tests

import (
	"errors"
	"net"
	"os"
	"os/exec"
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
func lookFound(string) (string, error)   { return "/usr/bin/Xvfb", nil }
func lookMissing(string) (string, error) { return "", exec.ErrNotFound }

func TestPlanDisplayReportsExistingAttachedAndXvfb(t *testing.T) {
	dir := shortDir(t)
	listenX(t, dir, "3")

	plan, err := cua.PlanDisplayForTest(":3", dir, os.Getuid(), lookMissing)
	if err != nil || plan.Source != "existing" || plan.Display != ":3" {
		t.Fatalf("existing: %+v, %v", plan, err)
	}
	plan, err = cua.PlanDisplayForTest("", dir, os.Getuid(), lookMissing)
	if err != nil || plan.Source != "attached" || plan.Display != ":3" {
		t.Fatalf("attached: %+v, %v", plan, err)
	}

	empty := shortDir(t)
	plan, err = cua.PlanDisplayForTest("", empty, os.Getuid(), lookFound)
	if err != nil || plan.Source != "xvfb" || plan.Display != "" {
		t.Fatalf("xvfb: %+v, %v", plan, err)
	}
}

func TestPlanDisplayWithNothingAvailableNamesTheFix(t *testing.T) {
	_, err := cua.PlanDisplayForTest("", shortDir(t), os.Getuid(), lookMissing)
	if !errors.Is(err, cua.ErrXvfbMissing) || !errors.Is(err, cua.ErrNoDisplay) {
		t.Fatalf("want ErrXvfbMissing wrapping ErrNoDisplay, got %v", err)
	}
}
