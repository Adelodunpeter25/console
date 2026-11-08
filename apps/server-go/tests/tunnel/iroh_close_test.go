package tests

import (
	"context"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/irohbackend"
)

func TestIrohCloseDoesNotWaitForAPeer(t *testing.T) {
	m := &irohbackend.Manager{Dir: t.TempDir(), Target: "127.0.0.1:1"}
	started := make(chan struct{})
	m.OnReady = func(string) { close(started) }
	go func() { _ = m.Start(context.Background()) }()
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("never ready")
	}
	t0 := time.Now()
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	select {
	case <-done:
		t.Logf("Close returned in %s", time.Since(t0))
	case <-time.After(8 * time.Second):
		t.Fatal("Close still blocked after 8s")
	}
}
