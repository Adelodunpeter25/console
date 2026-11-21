package tests

import (
	"context"
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
