// Verifies bash background jobs feed their output into the PortRegistry
// (mirrors bash/manager.ts's portRegistry.observeOutput/removeOwner wiring)
// so a dev server started via the bash tool gets auto-detected the same
// way the TS server does.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBashJobOutputRegistersPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	registry := services.NewPortRegistry()
	jobs := services.NewBashJobManager(registry)
	bash := tools.NewBashTool(jobs, "sess1")

	// The job must still be running when detection fires: maybeFinish
	// calls RemoveOwner the instant a job exits (mirrors bash/manager.ts),
	// so an already-exited "echo" job's port would already be gone by the
	// time this test observes it.
	startArgs, _ := json.Marshal(map[string]any{
		"command":    fmt.Sprintf("echo 'Server running at http://localhost:%d' && sleep 2", port),
		"background": true,
	})
	if _, err := bash.Execute(context.Background(), startArgs); err != nil {
		t.Fatalf("bash execute: %v", err)
	}

	if !waitForPort(registry, port, 2*time.Second) {
		t.Fatalf("port %d was not auto-detected from bash job output", port)
	}
}

func TestPortRegistryRemovesOwnerOnExit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	owner := services.PortOwner{Kind: "job", ID: "job_test"}

	registry := services.NewPortRegistry()
	registry.ObserveOutput(owner, fmt.Sprintf("localhost:%d", port), "")
	if !waitForPort(registry, port, 2*time.Second) {
		ln.Close()
		t.Fatal("setup: port was never detected")
	}

	ln.Close()
	registry.RemoveOwner(owner)

	for _, entry := range registry.List("localhost", "") {
		if entry.Port == port {
			t.Fatalf("port %d still registered after RemoveOwner", port)
		}
	}
}

func waitForPort(registry *services.PortRegistry, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, entry := range registry.List("localhost", "") {
			if entry.Port == port {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
