// Package gitconfig teaches the git CLI about the GitHub credential helper
// for this process and, transitively, every child process it spawns.
//
// Configuration rides the GIT_CONFIG_COUNT / GIT_CONFIG_KEY_n /
// GIT_CONFIG_VALUE_n env channel (git >= 2.31) rather than ~/.gitconfig, so
// nothing global is written and a disconnect leaves no trace on disk.
//
// Injecting into the server's own environment — instead of per-call-site env
// assembly — is what makes this complete. Every git path already inherits
// os.Environ():
//   - services.runGit (GitService status/diff/branches)
//   - agent bash tool, both modes, via tools.mergedEnv -> SpawnCapture and
//     BashJobManager.Start
//   - services.PtyManager.Spawn, via baseEnv()
//
// so one os.Setenv block covers all of them plus any call site added later.
//
// ORDERING: Configure must run before the first PTY spawn. PtyManager caches
// os.Environ() in a sync.Once (its own comment: "the server env is static
// after boot"), so a later Setenv silently misses every terminal. Call it
// first thing in serve.Run.
package gitconfig

import (
	"fmt"
	"os"
	"strings"
)

// HelperEnvVar marks a process as a git credential-helper invocation. The
// credential.helper value sets it inline, so it is present only when git is
// the caller.
const HelperEnvVar = "CONSOLE_GIT_CREDENTIAL_HELPER"

// gitConfigEntries returns the config triples to inject, in order. Each
// becomes one GIT_CONFIG_COUNT slot. The helper value needs the running
// binary's path, so this is a function of exe rather than a package var.
func gitConfigEntries(exe string) []struct{ key, value string } {
	return []struct{ key, value string }{
		// `!` marks a shell command helper. The env prefix selects
		// helper mode in cmd/console/main.go; without it the invocation
		// would be ambiguous with the daemon's own CONSOLE_SERVE=1 mode,
		// which this process inherits. The path is quoted because install
		// directories can contain spaces.
		{"credential.helper", "!" + HelperEnvVar + "=1 " + shellQuote(exe)},
		{"url.https://github.com/.insteadOf", "git@github.com:"},
		{"url.https://github.com/.insteadOf", "ssh://git@github.com/"},
	}
}

// Configure points git at this binary's credential-helper mode and rewrites
// SSH-style GitHub URLs to HTTPS, so a pasted git@github.com:org/repo.git
// works over a PAT with no SSH keys present.
//
// The insteadOf rewrite is not optional in practice: agents emit scp-style SSH
// remotes by default, which would otherwise fail "Permission denied
// (publickey)" and defeat the point of the feature.
//
// Failure to resolve the executable is returned rather than swallowed — the
// caller decides whether a server without git auth should still boot. When
// ConfigureWithExe returns an error the environment is left untouched.
//
// Configure resolves this binary's own path and applies gitConfigEntries.
func Configure() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve console binary for the git credential helper: %w", err)
	}
	return ConfigureWithExe(exe)
}

// ConfigureWithExe applies the config with an explicit helper binary path.
// The path is the only injectable part, so tests can drive the real git
// binary against a purpose-built `console` instead of the test binary.
func ConfigureWithExe(exe string) error {
	if strings.TrimSpace(exe) == "" {
		return fmt.Errorf("resolve console binary for the git credential helper: empty path")
	}

	entries := gitConfigEntries(exe)

	if err := os.Setenv("GIT_CONFIG_COUNT", fmt.Sprint(len(entries))); err != nil {
		return err
	}
	for i, entry := range entries {
		if err := os.Setenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", i), entry.key); err != nil {
			return err
		}
		if err := os.Setenv(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i), entry.value); err != nil {
			return err
		}
	}
	return nil
}

// shellQuote wraps a path in single quotes for /bin/sh, which is the shell
// git uses to run a `!` helper. Embedded single quotes are closed, escaped,
// and reopened.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
