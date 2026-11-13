// Environment / workstation info for the system prompt.
package systemprompt

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvironmentInfo is the host/environment summary shown in the workstation
// block of the system prompt.
type EnvironmentInfo struct {
	Date      string
	Cwd       string
	OS        string
	Arch      string
	GitBranch string
	Model     string
}

func gitBranch(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

// Today returns the date as it appears in the workstation block. Shared with
// the run service's prompt cache, which keys on it so a multi-day session
// picks up the new date on the first message after midnight.
func Today() string {
	return time.Now().Format("2006-01-02")
}

// CollectEnvironmentInfo gathers date/cwd/OS/arch/git-branch for cwd.
func CollectEnvironmentInfo(cwd, model string) EnvironmentInfo {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	return EnvironmentInfo{
		Date:      Today(),
		Cwd:       abs,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GitBranch: gitBranch(abs),
		Model:     model,
	}
}
