// Binary integration detection (/api/integrations). Each entry is a tool the
// server shells out to that must be installed on the box to work —
// agent-spawned `gh`, project scripts, terminals alike.
//
// Detection runs live on every read and is never cached: installing the
// binary then refreshing the page flips the status with no restart. The
// version probe has a hard timeout so a hung binary cannot wedge the
// request. Plain data in, plain data out — the proto conversion lives with
// the other wire converters in internal/routes.
package integrations

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ghVersionTimeout bounds the `gh --version` probe. Version output is
// informational; a slow binary must not slow the status page.
const ghVersionTimeout = 5 * time.Second

// IntegrationInfo is the detector's plain-Go shape. Mirrors
// console.v1.IntegrationStatus one-to-one; empty Version/Path mean absent.
type IntegrationInfo struct {
	ID          string
	Name        string
	Description string
	Installed   bool
	Version     string
	Path        string
	InstallURL  string
	InstallHint string
}

// ListIntegrations reports every known binary integration. One row today
// (gh); new binaries append here and the endpoint, clients, and tests pick
// them up with no other changes.
func ListIntegrations() []IntegrationInfo {
	return []IntegrationInfo{CheckGhCLI()}
}

// ghFirstLine is `gh version 2.75.0 (2025-11-19)` — the group after
// "version" is the whole signal.
var ghVersionRe = regexp.MustCompile(`(?m)^gh version (\S+)`)

// CheckGhCLI detects the GitHub CLI. The agent reaches for `gh` by default
// (PRs, CI status, releases) and GH_TOKEN pre-auth is already injected into
// every spawned command — but none of that matters when the binary itself is
// missing, which previously surfaced only as `command not found` mid-turn.
func CheckGhCLI() IntegrationInfo {
	info := IntegrationInfo{
		ID:          "gh",
		Name:        "GitHub CLI",
		Description: "Used by the agent to create pull requests, read CI status, and manage releases without a browser login.",
		InstallURL:  "https://cli.github.com",
		InstallHint: ghInstallHint(),
	}
	path, err := exec.LookPath("gh")
	if err != nil {
		// The Linux installer targets ~/.local/bin, which a thin daemon
		// PATH may not carry — so check it explicitly rather than reporting
		// a just-installed binary as missing.
		if userGh, statErr := os.Stat(filepath.Join(UserBinDir(), "gh")); statErr != nil || userGh.IsDir() {
			return info
		}
		path = filepath.Join(UserBinDir(), "gh")
	}
	info.Installed = true
	info.Path = path
	if v, err := ghVersion(path); err == nil {
		info.Version = v
	}
	// A version probe failure keeps installed=true: the binary resolves and
	// runs, which is the actual requirement. An empty version simply reads
	// as absent on the wire.
	return info
}

// ghVersion runs `<path> --version` and parses the first line. Separate from
// the lookup so a present-but-broken binary still reports installed.
func ghVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ghVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	m := ghVersionRe.FindStringSubmatch(string(out))
	if m == nil {
		return "", errGhVersionUnparsed
	}
	return strings.TrimSpace(m[1]), nil
}

// errGhVersionUnparsed marks output that ran fine but matched no version
// line. Callers ignore it and report installed with no version.
var errGhVersionUnparsed = errors.New("gh version output did not match")

// ghInstallHint is the copy-paste command for the server's OS. Only the
// common cases are covered; anything else falls back to InstallURL.
func ghInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install gh"
	case "windows":
		return "winget install GitHub.cli"
	default:
		return "sudo apt install gh"
	}
}
