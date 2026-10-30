//go:build linux

package tools

import "os/exec"

// Linux: bubblewrap mounts the whole filesystem read-only and gives the temp
// folders back as writable. The network namespace is left alone, so the network
// stays open. Needs `bwrap` installed and unprivileged user namespaces enabled;
// the probe in SandboxAvailable verifies both before the sandbox is used.
func init() {
	sandboxArgv = func(argv []string) ([]string, bool) {
		path, err := exec.LookPath("bwrap")
		if err != nil {
			return argv, false
		}
		args := []string{
			path,
			"--ro-bind", "/", "/",
			"--dev", "/dev",
			"--proc", "/proc",
			"--bind", "/tmp", "/tmp",
			"--bind-try", "/var/tmp", "/var/tmp",
			"--die-with-parent",
			"--",
		}
		return append(args, argv...), true
	}
}
