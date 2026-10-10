// Coverage for the ignored-path list that backs file browsing, tree walking,
// search, and worktree file carry-over. Matching is per whole path segment and
// case-insensitive, so the cases below pin those two properties as much as the
// individual names.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func TestIsPathIgnoredMatchesWholeSegments(t *testing.T) {
	for _, path := range []string{
		".cxx",
		"app/.cxx",
		"app/.cxx/Debug/obj",
		"APP/.CXX",
		// Android's NDK build tree, which sits next to .cxx.
		"android/app/.externalNativeBuild",
		// Kotlin compiler session cache.
		"apps/android/.kotlin/sessions",
		// Entries that were already present must keep working.
		"node_modules/pkg/index.js",
		"services/.git/config",
		"apps/desktop/target/debug/console",
		"app/build/outputs/apk",
		"pkg/__pycache__/mod.pyc",
	} {
		if !utils.IsPathIgnored(path) {
			t.Errorf("IsPathIgnored(%q) = false, want true", path)
		}
	}
}

func TestIsPathIgnoredKeepsSourcePaths(t *testing.T) {
	for _, path := range []string{
		"",
		"src",
		"src/main.rs",
		"apps/server-go/internal/utils/paths.go",
		// Suffix/substring matches must not count: ".cxx" must not swallow
		// "my.cxxdir" or "cxx".
		"cxx",
		"my.cxxdir",
		"vendor",
		"README.md",
	} {
		if utils.IsPathIgnored(path) {
			t.Errorf("IsPathIgnored(%q) = true, want false", path)
		}
	}
}

// IsHiddenName is the separate dotfile rule, checked here because the two
// predicates sit next to each other and callers mix them up.
func TestIsHiddenName(t *testing.T) {
	if !utils.IsHiddenName(".env") {
		t.Error(".env must count as hidden")
	}
	if utils.IsHiddenName("src") {
		t.Error("src must not count as hidden")
	}
}