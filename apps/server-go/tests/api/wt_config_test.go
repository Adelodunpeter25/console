// Transport config coverage (QUIC/WebTransport migration, Phase 0):
// mode parsing, TLS gating, and defaults.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

func wtEnv(t *testing.T, mode, addr, cert, key string) {
	t.Helper()
	t.Setenv("CONSOLE_TRANSPORT", mode)
	t.Setenv("CONSOLE_WT_ADDR", addr)
	t.Setenv("CONSOLE_TLS_CERT", cert)
	t.Setenv("CONSOLE_TLS_KEY", key)
	t.Setenv("CONSOLE_WT_QLOG_DIR", "")
}

func TestWTConfigDefaultsToAutoLegacyWithoutTLS(t *testing.T) {
	wtEnv(t, "", "", "", "")
	cfg, err := wt.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != wt.TransportAuto || cfg.WTAddr != ":443" {
		t.Fatalf("defaults: %+v", cfg)
	}
	if cfg.WTEnabled() {
		t.Fatal("WT must stay off without TLS material")
	}
}

func TestWTConfigUnknownModeFallsBackToAuto(t *testing.T) {
	wtEnv(t, "hyperloop", "", "", "")
	cfg, err := wt.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != wt.TransportAuto {
		t.Fatalf("mode = %q", cfg.Mode)
	}
}

func TestWTConfigEnablesWithTLS(t *testing.T) {
	wtEnv(t, "auto", ":8443", "/tmp/cert.pem", "/tmp/key.pem")
	cfg, err := wt.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WTEnabled() || cfg.WTAddr != ":8443" {
		t.Fatalf("TLS should enable WT: %+v", cfg)
	}
}

func TestWTConfigLegacyNeverEnables(t *testing.T) {
	wtEnv(t, "legacy", "", "/tmp/cert.pem", "/tmp/key.pem")
	cfg, err := wt.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WTEnabled() {
		t.Fatalf("legacy must stay off: %+v", cfg)
	}
}

func TestWTConfigWtModeRequiresTLS(t *testing.T) {
	wtEnv(t, "wt", "", "", "")
	if _, err := wt.ConfigFromEnv(); err == nil {
		t.Fatal("wt mode without TLS must fail")
	}
	wtEnv(t, "wt", "", "/tmp/cert.pem", "/tmp/key.pem")
	cfg, err := wt.ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WTEnabled() {
		t.Fatalf("wt mode with TLS must enable: %+v", cfg)
	}
}
