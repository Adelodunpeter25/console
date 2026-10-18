package tests

import (
	"os"
	"testing"
)

// TestMain isolates the credential stores for the whole package.
//
// The provider tests exercise code that resolves real credentials, and the
// default resolution path reads the developer's live store at
// ~/.console/provider-keys.json. Without this, running the suite on a
// developer machine silently uses the developer's actual provider key as the
// test key — and on failure, prints it into the test log alongside the
// assertion that rejected it.
//
// That is not hypothetical: it happened once already, and the key landed in a
// test output file on disk. Point both overrides at a temp dir so the suite can
// only ever see keys a test created itself.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "console-provider-tests-")
	if err != nil {
		panic("cannot create isolated credential dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	// Pasted provider API keys (opencode and friends).
	os.Setenv("CONSOLE_PROVIDER_KEYS_PATH", dir+"/provider-keys.json")
	// Provider env overrides. A developer's shell exports must not decide what
	// the suite tests, so clear first...
	os.Unsetenv("OPENCODE_API_KEY")
	// ...then set one deliberately. The provider now requires a credential and
	// refuses to build a request without it, so a package-wide default is
	// clearer than repeating the setup in every test that reaches the network.
	// Individual tests override the value with t.Setenv where it matters.
	os.Setenv("OPENCODE_API_KEY", "oc_sk_test_key")

	os.Exit(m.Run())
}
