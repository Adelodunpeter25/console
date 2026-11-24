package tests

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/computer"
)

func TestComputerServiceMissingLibvoxl(t *testing.T) {
	t.Setenv("VOXL_LIB_PATH", "/nonexistent/libvoxl.so")
	svc := computer.New()
	defer svc.Close()

	_, err := svc.Subscribe(context.Background(), func(uint32, []byte) {})
	if err == nil {
		t.Fatal("expected error when libvoxl is missing, got nil")
	}
}

func TestResolveSessionBus(t *testing.T) {
	if got := computer.ResolveSessionBus("unix:path=/custom/bus", "/tmp"); got != "unix:path=/custom/bus" {
		t.Fatalf("expected existing bus address to win, got %q", got)
	}

	dir := t.TempDir()
	if got := computer.ResolveSessionBus("", dir); got != "" {
		t.Fatalf("expected empty string when no bus socket exists, got %q", got)
	}

	sockPath := filepath.Join(dir, "bus")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer ln.Close()

	want := "unix:path=" + sockPath
	if got := computer.ResolveSessionBus("", dir); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
