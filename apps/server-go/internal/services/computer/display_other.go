//go:build !linux

package computer

import "context"

type displayManager struct{}

func newDisplayManager() *displayManager {
	return &displayManager{}
}

func (d *displayManager) EnsureDisplay(ctx context.Context) (string, error) {
	// On macOS or Windows, voxl captures the active screen directly.
	return "", nil
}

func (d *displayManager) Close() {}
