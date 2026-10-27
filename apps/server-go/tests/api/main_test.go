package tests

import (
	"os"
	"testing"
)

// TestMain isolates the credential store for the whole package.
//
// The provider-credential endpoints resolve real credentials, and the default
// path reads the developer's live store at ~/.console/provider-keys.json.
// Individual tests set their own temp path via useTempKeyStore, but doing it
// package-wide means a test that forgets cannot reach the real key — and on a
// failing assertion, cannot print it into the test log.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "console-api-tests-")
	if err != nil {
		panic("cannot create isolated credential dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	os.Setenv("CONSOLE_PROVIDER_KEYS_PATH", dir+"/provider-keys.json")
	// OAuth credential paths: the account-store migration (internal/accounts)
	// reads these to import legacy single-credential files. Pointing them at
	// files that do not exist means a test cannot ingest the developer's
	// ~/.console credentials; tests that need a legacy fixture set their own.
	os.Setenv("CLAUDE_CREDENTIALS_PATH", dir+"/claude-creds.json")
	os.Setenv("CODEX_CREDENTIALS_PATH", dir+"/codex-creds.json")
	os.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", dir+"/antigravity-creds.json")
	// No ambient key from the developer's shell; tests that need one set it
	// explicitly with t.Setenv.
	os.Unsetenv("OPENCODE_API_KEY")

	os.Exit(m.Run())
}