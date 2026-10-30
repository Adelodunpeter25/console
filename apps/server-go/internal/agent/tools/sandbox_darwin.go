//go:build darwin

package tools

import "os/exec"

// macOS: sandbox-exec with a profile that allows everything except file writes,
// then re-allows writes to the temp folders and the terminal devices. Apple
// marks sandbox-exec deprecated, but it still ships and the mechanism behind it
// is what Chrome and Homebrew rely on.
const darwinProfile = `(version 1)
(allow default)
(deny file-write*)
(allow file-write*
  (subpath "/private/tmp")
  (subpath "/private/var/tmp")
  (subpath "/private/var/folders")
  (literal "/dev/null")
  (literal "/dev/dtracehelper")
  (regex #"^/dev/tty")
  (regex #"^/dev/fd/"))`

func init() {
	sandboxArgv = func(argv []string) ([]string, bool) {
		path, err := exec.LookPath("sandbox-exec")
		if err != nil {
			return argv, false
		}
		return append([]string{path, "-p", darwinProfile}, argv...), true
	}
}
