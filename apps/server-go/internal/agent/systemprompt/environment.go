// Environment / workstation info for the system prompt. Port of
// apps/server/agent/src/systemprompt/environment.ts.
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

// CollectEnvironmentInfo gathers date/cwd/OS/arch/git-branch for cwd.
func CollectEnvironmentInfo(cwd, model string) EnvironmentInfo {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	return EnvironmentInfo{
		Date:      time.Now().Format("2006-01-02"),
		Cwd:       abs,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GitBranch: gitBranch(abs),
		Model:     model,
	}
}
