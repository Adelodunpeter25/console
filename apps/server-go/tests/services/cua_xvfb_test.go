// Managed Xvfb: startup reads the display the server chose, a server that dies
// early is reported, and Stop reaps the process. A shell script stands in for
// Xvfb so the tests need no X server.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func fakeXvfb(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Xvfb")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestXvfbReportsChosenDisplayAndStops(t *testing.T) {
	bin := fakeXvfb(t, "echo 101 >&3\nexec sleep 30")
	display, stop, err := cua.StartXvfbForTest(bin, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if display != ":101" {
		t.Errorf("display = %q, want :101", display)
	}
	stop()
	stop() // idempotent
}

func TestXvfbExitingEarlyIsAnError(t *testing.T) {
	bin := fakeXvfb(t, "exit 1")
	_, _, err := cua.StartXvfbForTest(bin, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "before reporting") {
		t.Fatalf("want early-exit error, got %v", err)
	}
}

func TestXvfbThatNeverReportsTimesOut(t *testing.T) {
	bin := fakeXvfb(t, "exec sleep 30")
	_, _, err := cua.StartXvfbForTest(bin, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "in time") {
		t.Fatalf("want timeout error, got %v", err)
	}
}

func TestXvfbMissingErrorNamesThePackage(t *testing.T) {
	if !strings.Contains(cua.ErrXvfbMissing.Error(), "apt install xvfb") {
		t.Fatalf("message should name the package: %v", cua.ErrXvfbMissing)
	}
}
