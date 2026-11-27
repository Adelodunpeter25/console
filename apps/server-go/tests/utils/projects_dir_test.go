// The projects root follows the WorktreesDir split: prod default, dev
// variant, explicit override. The picker's server-chosen clone destination
// depends on this.
package tests

import (
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func TestProjectsDirCentralized(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONSOLE_ENV", "")
	t.Setenv("CONSOLE_PROJECTS_DIR", "")

	if got := utils.ProjectsDir(); got != filepath.Join(home, "console", "projects") {
		t.Fatalf("prod root = %q", got)
	}
	t.Setenv("CONSOLE_ENV", "dev")
	if got := utils.ProjectsDir(); got != filepath.Join(home, "console-dev", "projects") {
		t.Fatalf("dev root = %q", got)
	}
	t.Setenv("CONSOLE_PROJECTS_DIR", filepath.Join(home, "custom-projects"))
	if got := utils.ProjectsDir(); got != filepath.Join(home, "custom-projects") {
		t.Fatalf("override root = %q", got)
	}
}
