// Persistent per-install identity, used to derive stable device ids for
// providers that need one (e.g. Claude's OAuth metadata.user_id).
package utils

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const installIDFile = "install-id"

var uuidShapePattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var (
	installIDOnce  sync.Once
	installIDValue string
)

// InstallID returns this machine's persistent per-install identifier,
// generating and persisting one at <ConsoleStorageDir>/install-id on first
// use. It survives independently of any project/session state.
func InstallID() string {
	installIDOnce.Do(func() {
		installIDValue = loadOrCreateInstallID()
	})
	return installIDValue
}

func loadOrCreateInstallID() string {
	path := filepath.Join(ConsoleStorageDir(), installIDFile)
	if data, err := os.ReadFile(path); err == nil {
		if existing := strings.TrimSpace(string(data)); uuidShapePattern.MatchString(strings.ToLower(existing)) {
			return existing
		}
	}
	next := RandomID()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	// Best-effort persistence: a concurrent first-run writer racing this one
	// only means two processes briefly disagree on the id, which is
	// harmless for a soft device-identity hint (not a security boundary).
	_ = os.WriteFile(path, []byte(next), 0o644)
	return next
}
