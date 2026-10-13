// Transport configuration for the QUIC/WebTransport migration.
//
// Server side only needs "am I serving H3" plus listener addresses;
// transport selection (auto|wt|legacy) is a client-side concept carried here
// so both sides read one vocabulary.
package wt

import (
	"fmt"
	"os"
	"strings"
)

// TransportMode selects which transports are active. On the server, `auto`
// serves WebTransport whenever TLS is configured and otherwise stays legacy;
// `wt` requires TLS (boot fails without it); `legacy` never serves WT.
type TransportMode string

const (
	TransportAuto   TransportMode = "auto"
	TransportWT     TransportMode = "wt"
	TransportLegacy TransportMode = "legacy"
)

// DefaultWTAddr is the dual-stack port: TCP (Fiber/TLS) + UDP (HTTP/3).
const DefaultWTAddr = ":443"

// Config is the resolved transport surface.
type Config struct {
	Mode        TransportMode
	WTAddr      string
	TLSCertFile string
	TLSKeyFile  string
	// QlogDir enables qlog tracing when set (Phase 2+).
	QlogDir string
}

// WTEnabled reports whether the WT listener should start.
func (c Config) WTEnabled() bool {
	return c.Mode != TransportLegacy && c.TLSCertFile != "" && c.TLSKeyFile != ""
}

// ConfigFromEnv reads the transport surface. Unknown modes fall back to
// auto; `wt` mode without TLS paths is a boot error.
func ConfigFromEnv() (Config, error) {
	mode := TransportMode(strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_TRANSPORT"))))
	switch mode {
	case TransportWT, TransportLegacy:
	default:
		mode = TransportAuto
	}
	cfg := Config{
		Mode:        mode,
		WTAddr:      os.Getenv("CONSOLE_WT_ADDR"),
		TLSCertFile: strings.TrimSpace(os.Getenv("CONSOLE_TLS_CERT")),
		TLSKeyFile:  strings.TrimSpace(os.Getenv("CONSOLE_TLS_KEY")),
		QlogDir:     strings.TrimSpace(os.Getenv("CONSOLE_WT_QLOG_DIR")),
	}
	if cfg.WTAddr == "" {
		cfg.WTAddr = DefaultWTAddr
	}
	if mode == TransportWT && (cfg.TLSCertFile == "" || cfg.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("CONSOLE_TRANSPORT=wt requires CONSOLE_TLS_CERT and CONSOLE_TLS_KEY.")
	}
	return cfg, nil
}
