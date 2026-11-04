// Package tunnel picks which remote-access backend the server uses.
//
// The relay Worker in apps/relay (package cfrelay) is the default. Setting
// CONSOLE_RELAY=0 switches back to OpenTunnel.
package tunnel

import (
	"os"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/cfrelay"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
)

// RelayEnabled reports whether the relay is the backend. It is unless the
// operator chose OpenTunnel with CONSOLE_RELAY=0 (or off, false, no, opentunnel).
func RelayEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_RELAY"))) {
	case "0", "off", "false", "no", "disabled", "opentunnel":
		return false
	}
	return true
}

// Info is the public address of the active backend, as the CLI shows it.
type Info struct {
	URL string
	// Ready is false while an OpenTunnel certificate is still being issued.
	Ready bool
}

// ReadInfo reports the active backend's stored address in dir, if any.
func ReadInfo(dir string) (Info, bool) {
	if RelayEnabled() {
		info, ok := cfrelay.ReadInfo(dir)
		return Info{URL: info.URL, Ready: info.Ready}, ok
	}
	info, ok := opentunnel.ReadInfo(dir)
	return Info{URL: info.URL, Ready: info.Ready}, ok
}
