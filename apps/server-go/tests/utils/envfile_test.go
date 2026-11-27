// The env file is the one place a per-machine setting survives a restart, so
// the rules about it are pinned here: the file fills gaps in the environment
// and never overrides a variable that was set explicitly.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func TestParseEnvFileUnwrapsQuotesAndSkipsNoise(t *testing.T) {
	parsed := utils.ParseEnvFile(
		"# a comment\n" +
			"\n" +
			"CONSOLE_HARNESS_CODEMODE=1\n" +
			"FIRECRAWL_API_KEY=\"fc-abc\"\n" +
			"QUOTED='plain'\n" +
			"SPACED = value \n" +
			"no-equals-here\n",
	)
	if got := parsed["CONSOLE_HARNESS_CODEMODE"]; got != "1" {
		t.Fatalf("CONSOLE_HARNESS_CODEMODE = %q, want %q", got, "1")
	}
	if got := parsed["FIRECRAWL_API_KEY"]; got != "fc-abc" {
		t.Fatalf("FIRECRAWL_API_KEY = %q, want %q", got, "fc-abc")
	}
	if got := parsed["QUOTED"]; got != "plain" {
		t.Fatalf("QUOTED = %q, want %q", got, "plain")
	}
	if got := parsed["SPACED"]; got != "value" {
		t.Fatalf("SPACED = %q, want %q", got, "value")
	}
	if _, ok := parsed["no-equals-here"]; ok {
		t.Fatal("a line without '=' must not become an entry")
	}
}

func TestApplyEnvFileFillsGapsAndNeverOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_STORAGE_DIR", dir)
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("CONSOLE_HARNESS_CODEMODE=1\nALREADY_SET=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY_SET", "from-process")

	utils.ApplyEnvFile()

	if got := os.Getenv("CONSOLE_HARNESS_CODEMODE"); got != "1" {
		t.Fatalf("CONSOLE_HARNESS_CODEMODE = %q, want %q", got, "1")
	}
	if got := os.Getenv("ALREADY_SET"); got != "from-process" {
		t.Fatalf("ALREADY_SET = %q, want %q — the process environment wins", got, "from-process")
	}
}

func TestApplyEnvFileWithNoFileIsHarmless(t *testing.T) {
	t.Setenv("CONSOLE_STORAGE_DIR", t.TempDir())
	utils.ApplyEnvFile()
}
