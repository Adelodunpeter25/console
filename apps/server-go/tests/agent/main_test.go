package tests

import (
	"os"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

// TestMain isolates the credential stores and seeds OpenCode capabilities for
// the whole package.
//
// Two reasons, both learned the hard way:
//
//  1. Isolation. The run pipeline resolves real credentials, and the default
//     resolution path reads the developer's live store at
//     ~/.console/provider-keys.json. Without this the suite silently uses the
//     developer's actual provider key as the test key — and prints it into the
//     test log whenever a header assertion fails.
//
//  2. Capabilities. OpenCode effort levels used to come from a hardcoded table
//     keyed by model-id prefix. They now come from models.dev, so tests that
//     assert level forwarding have to say what those vocabularies are — which
//     is more honest anyway: it states the premise instead of relying on a
//     table the production code no longer contains.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "console-agent-tests-")
	if err != nil {
		panic("cannot create isolated credential dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	os.Setenv("CONSOLE_PROVIDER_KEYS_PATH", dir+"/provider-keys.json")
	os.Setenv("OPENCODE_API_KEY", "oc_sk_test_key")

	// The two vocabularies the OpenCode role tests distinguish between:
	// space-bunny runs one step further than muse-spark, so "max" is valid for
	// the first and invalid for the second, and muse-spark offers a "minimal"
	// step that space-bunny does not.
	opencode.UseMetadata(map[string]opencode.ModelMetadata{
		"space-bunny": {
			ContextWindow:  1_000_000,
			SupportsImages: true,
			ThinkingLevels: []string{"low", "medium", "high", "xhigh", "max"},
		},
		"muse-spark-1.3-contributor": {
			ContextWindow:  200_000,
			SupportsImages: false,
			ThinkingLevels: []string{"minimal", "low", "medium", "high", "xhigh"},
		},
	})

	os.Exit(m.Run())
}
