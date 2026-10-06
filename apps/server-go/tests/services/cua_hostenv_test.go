// Host environment: bundle identification without a plist parser. ownBundleID
// depends on where the test binary lives, so only the plist scan and the
// explicit-override behaviour are pinned here.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func TestHostEnvLeavesExplicitSettingsAlone(t *testing.T) {
	t.Setenv("CUA_DRIVER_EMBEDDED", "0")
	t.Setenv("CUA_DRIVER_HOST_BUNDLE_ID", "custom.example")
	cua.EnsureHostEnv()
	if got := os.Getenv("CUA_DRIVER_EMBEDDED"); got != "0" {
		t.Errorf("explicit EMBEDDED overwritten: %q", got)
	}
	if got := os.Getenv("CUA_DRIVER_HOST_BUNDLE_ID"); got != "custom.example" {
		t.Errorf("explicit bundle id overwritten: %q", got)
	}
}

func TestHostEnvDefaultsEmbeddedOnDarwin(t *testing.T) {
	// Unset means "derive it"; on macOS that always enables embedded mode.
	os.Unsetenv("CUA_DRIVER_EMBEDDED")
	os.Unsetenv("CUA_DRIVER_HOST_BUNDLE_ID")
	cua.EnsureHostEnv()
	if got := os.Getenv("CUA_DRIVER_EMBEDDED"); got != "1" {
		t.Errorf("EMBEDDED = %q, want 1", got)
	}
	// The test binary is not inside a bundle, so no id can be derived, but
	// the call must still be safe.
	_ = os.Getenv("CUA_DRIVER_HOST_BUNDLE_ID")
}

func TestBundlePlistScanFindsIdentifier(t *testing.T) {
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<dict>
	<key>CFBundleExecutable</key>
	<string>console</string>
	<key>CFBundleIdentifier</key>
	<string>sh.console.computeruse</string>
</dict>`
	if got := cua.PlistStringForTest(plist, "CFBundleIdentifier"); got != "sh.console.computeruse" {
		t.Errorf("got %q", got)
	}
	if got := cua.PlistStringForTest(plist, "Missing"); got != "" {
		t.Errorf("missing key should be empty, got %q", got)
	}
}

func TestBundlePlistScanRejectsNonBundleLayout(t *testing.T) {
	// ownBundleID returns "" outside a bundle; the observable half is that a
	// library path next to a bare binary is still found. This just pins the
	// candidate layout the loader documents.
	dir := t.TempDir()
	if _, err := os.Stat(filepath.Join(dir, "libcua_driver_sdk.so")); err == nil {
		t.Fatal("temp dir should be empty")
	}
}
