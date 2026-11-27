// Environment / workstation info for the system prompt.
package systemprompt

import (
	"path/filepath"
	"runtime"
)

// EnvironmentInfo is the host/environment summary shown in the workstation
// block of the system prompt. Only session-stable facts belong here: the
// date and git branch are omitted on purpose — they go stale in a cached
// prompt, and the model can look both up with `date` and `git branch`
// whenever a task actually needs them.
type EnvironmentInfo struct {
	Cwd   string
	OS    string
	Arch  string
	Model string
}

// CollectEnvironmentInfo gathers cwd/OS/arch for the workstation block.
func CollectEnvironmentInfo(cwd, model string) EnvironmentInfo {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	return EnvironmentInfo{
		Cwd:   abs,
		OS:    runtime.GOOS,
		Arch:  runtime.GOARCH,
		Model: model,
	}
}
