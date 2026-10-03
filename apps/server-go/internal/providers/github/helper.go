// git credential-helper protocol implementation, reached through the
// `console git-credential-helper` CLI subcommand (see cli/root.go). git
// invokes it as `! <console> git-credential-helper get` via the
// credential.helper config entry that internal/gitconfig injects into the
// server process env.
//
// Why a helper rather than GIT_ASKPASS: git consults credential helpers
// before any prompt, in TTY and non-TTY contexts alike, so one mechanism
// covers agent-spawned git (non-interactive, would otherwise hang) and
// interactive PTY shells identically.
//
// Invoking this as a subcommand rather than a new multi-call env mode is
// deliberate: the daemon runs under CONSOLE_SERVE=1 and that variable is
// inherited by every child, so an env-triggered helper branch would have to
// be checked before the serve branch to avoid re-launching the daemon.
package github

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// githubHost is the only host v1 answers for. GHE is out of scope; extend by
// storing per-host credential entries later.
const githubHost = "github.com"

// HelperSubcommand is the CLI subcommand git invokes. It is the single source
// of truth for the name because two places must agree: the cobra command
// registration and the credential.helper config value built by
// internal/gitconfig.
const HelperSubcommand = "git-credential-helper"

// RunCredentialHelper handles one helper invocation and returns a process exit
// code. op is "get", "store", or "erase"; git passes it as argv[1].
//
// Protocol notes: on "get" git writes key=value lines to stdin terminated by
// a blank line and expects key=value lines back, also blank-terminated.
// Unknown keys (path, wwwauth[], ...) must be ignored rather than rejected.
// An empty response means "no opinion" and git falls through to its next
// helper or a normal prompt, which is exactly the behavior when no token is
// stored.
func RunCredentialHelper(op string, stdin io.Reader, stdout io.Writer) int {
	switch op {
	case "store", "erase":
		// The server owns the credential file. Accepting git's writes would
		// let any git command in a terminal overwrite or silently drop the
		// token, so these are acknowledged no-ops.
		return 0
	case "get":
		return credentialGet(stdin, stdout)
	default:
		// Unknown verb: stay silent rather than write junk into the protocol
		// stream, but signal failure so git does not treat us as a helper
		// that supplied credentials.
		return 1
	}
}

func credentialGet(stdin io.Reader, stdout io.Writer) int {
	fields, ok := parseCredentialRequest(stdin)
	if !ok || !fields["protocol"].isHTTPS() || !isGitHubHost(fields["host"].value) {
		return 0
	}
	cred, err := LoadCredential()
	if err != nil || strings.TrimSpace(cred.Token) == "" {
		return 0
	}
	// x-access-token is GitHub's documented username for token auth.
	fmt.Fprintf(stdout, "username=x-access-token\npassword=%s\n\n", cred.Token)
	return 0
}

// isHTTPS reports whether the protocol is acceptable. An absent protocol is
// tolerated (some git invocations and manual `git credential fill` omit it);
// anything other than https — notably ssh — is refused.
func (p credentialField) isHTTPS() bool {
	switch strings.ToLower(strings.TrimSpace(p.value)) {
	case "", "https":
		return true
	default:
		return false
	}
}

// isGitHubHost matches github.com exactly after lowercasing and dropping any
// :port. A suffix match would hand the token to lookalikes such as
// github.com.evil.test, so equality is the whole rule.
func isGitHubHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host == githubHost
}

// parseCredentialRequest reads git's blank-line-terminated key=value stdin.
func parseCredentialRequest(r io.Reader) (map[string]credentialField, bool) {
	fields := map[string]credentialField{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), 1<<16)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			// Blank line terminates the request.
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = credentialField{value: value}
	}
	if err := scanner.Err(); err != nil {
		return nil, false
	}
	// No host means there is nothing to match against.
	if _, ok := fields["host"]; !ok {
		return nil, false
	}
	return fields, true
}

type credentialField struct{ value string }
