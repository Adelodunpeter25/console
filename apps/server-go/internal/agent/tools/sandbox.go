// Read-only sandbox for bash in plan mode.
//
// Plan mode is read-only, but planning still wants `git log`, `grep`, `ls`, and
// the like. Rather than guess which command strings are harmless, the shell
// runs under an operating-system sandbox that refuses every file write outside
// the system temp folder. The kernel makes the call, so how a command is
// phrased (redirect, rm, a Python one-liner, git commit) makes no difference.
//
// The network stays open and reads are unrestricted, by decision. When no
// sandbox is available on the machine, plan mode runs bash normally.
package tools

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// SandboxMode says how a bash tool should treat its commands.
type SandboxMode int

const (
	// SandboxOff runs commands as given (every mode except plan mode).
	SandboxOff SandboxMode = iota
	// SandboxReadOnly runs commands under the read-only sandbox when this
	// machine has one, and as given when it does not.
	SandboxReadOnly
)

// sandboxArgv is the platform's wrapper: given the command's argv it returns
// the argv to run instead. ok is false when this machine cannot sandbox. It is
// set per platform (sandbox_darwin.go, sandbox_linux.go); the default reports
// unsupported.
var sandboxArgv = func(argv []string) (wrapped []string, ok bool) { return argv, false }

var (
	probeOnce   sync.Once
	probeResult bool
)

// SandboxAvailable reports whether the sandbox works here. It checks once, by
// really running a write under the sandbox and confirming it is refused, so a
// tool that exists but cannot run (namespaces disabled, a container) counts as
// unavailable instead of failing every plan-mode command.
func SandboxAvailable() bool {
	probeOnce.Do(func() { probeResult = probeSandbox() })
	return probeResult
}

func probeSandbox() bool {
	dir, err := os.MkdirTemp(homeForProbe(), ".console-sandbox-probe-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	target := dir + "/probe"

	// A read must work and a write outside temp must be refused.
	read, ok := sandboxArgv([]string{"/bin/sh", "-c", "cat /etc/hosts >/dev/null 2>&1 || cat /etc/passwd >/dev/null"})
	if !ok {
		return false
	}
	if err := exec.Command(read[0], read[1:]...).Run(); err != nil {
		return false
	}
	write, _ := sandboxArgv([]string{"/bin/sh", "-c", "echo x > " + shellQuote(target)})
	_ = exec.Command(write[0], write[1:]...).Run()
	if _, err := os.Stat(target); err == nil {
		return false // the write landed: this is not a working sandbox
	}
	return true
}

// homeForProbe picks a directory the probe write is aimed at that is NOT under
// the system temp folder, since temp is deliberately writable inside the sandbox.
func homeForProbe() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// sandboxedShellArgv is shellArgv for a bash tool in the given mode.
func sandboxedShellArgv(command string, mode SandboxMode) []string {
	argv := shellArgv(command)
	if mode != SandboxReadOnly || !SandboxAvailable() {
		return argv
	}
	if wrapped, ok := sandboxArgv(argv); ok {
		return wrapped
	}
	return argv
}

// sandboxHint is added to a failed command's output in plan mode, because
// "Operation not permitted" or "Read-only file system" alone reads as a bug.
const sandboxHint = "\n\nPlan mode runs commands in a read-only sandbox: they can read anything and write only to the system temp folder. If this command needed to write, describe the change in your plan instead, or ask the user to leave plan mode."

func looksLikeBlockedWrite(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "operation not permitted") ||
		strings.Contains(s, "read-only file system") ||
		strings.Contains(s, "permission denied")
}
