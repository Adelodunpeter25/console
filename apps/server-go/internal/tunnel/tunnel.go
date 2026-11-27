// Package tunnel picks which remote-access backend the server uses.
//
// iroh is the default wherever its libraries are available. OpenTunnel is
// the fallback, and CONSOLE_RELAY=0 (or opentunnel, off, false, no, disabled)
// selects it explicitly; CONSOLE_RELAY=iroh forces iroh.
package tunnel

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
)

// Backend names recorded in the tunnel dir. Pairing runs in another process,
// so the running server has to say which backend it actually started.
const (
	BackendIroh       = "iroh"
	BackendOpenTunnel = "opentunnel"
)

const backendFile = "backend"

// requestedBackend maps CONSOLE_RELAY to an explicit choice, or "" when the
// operator left it to the default.
func requestedBackend() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_RELAY"))) {
	case "iroh":
		return BackendIroh
	case "0", "off", "false", "no", "disabled", "opentunnel":
		return BackendOpenTunnel
	}
	return ""
}

// IrohSelected reports whether iroh is the backend: chosen explicitly, or the
// default when the platform has the iroh libraries.
func IrohSelected() bool {
	switch requestedBackend() {
	case BackendIroh:
		return true
	case BackendOpenTunnel:
		return false
	}
	return IrohAvailable
}

// Info is the public address of the active backend, as the CLI shows it.
type Info struct {
	URL string
	// NodeID is the iroh node ID; set only by the iroh backend.
	NodeID string
	// Ready is false while an OpenTunnel certificate is still being issued.
	Ready bool
}

// SetBackend records the backend the server actually started, so a pairing
// process can read the right address even after iroh fell back to OpenTunnel.
func SetBackend(dir, backend string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, backendFile), []byte(backend), 0o600)
}

// activeBackend returns the recorded backend, or "" when none was recorded.
func activeBackend(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, backendFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ReadInfo reports the active backend's stored address in dir, if any.
func ReadInfo(dir string) (Info, bool) {
	if backend := activeBackend(dir); backend != "" {
		return readBackend(dir, backend)
	}
	// Older servers did not record the backend; keep the selection rules as
	// the fallback.
	if IrohSelected() {
		return readBackend(dir, BackendIroh)
	}
	return readBackend(dir, BackendOpenTunnel)
}

// readBackend reads the address one specific backend wrote to dir.
func readBackend(dir, backend string) (Info, bool) {
	switch backend {
	case BackendIroh:
		return irohInfo(dir)
	case BackendOpenTunnel:
		info, ok := opentunnel.ReadInfo(dir)
		return Info{URL: info.URL, Ready: info.Ready}, ok
	}
	return Info{}, false
}
