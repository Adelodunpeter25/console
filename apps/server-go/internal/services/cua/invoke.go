// The /computer-use chat command: detection and task splitting.
//
// Typing "/computer-use <task>" activates computer use for the session. The
// model has no prior knowledge of it — no tool, no skill listing — so the
// command is matched here in the run path rather than discovered by the
// model. Matching is deliberately narrow: the command must open the message
// (after whitespace), followed by whitespace or the end of the message, so
// "/computer-useful" and inline mentions never trigger.
package cua

import "strings"

// commandPrefix is the leading command that activates computer use.
const commandPrefix = "/computer-use"

// SplitInvocation splits a "/computer-use <task>" message into its task. It
// reports false for anything else, including a bare mention mid-sentence.
func SplitInvocation(text string) (task string, ok bool) {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if !strings.HasPrefix(trimmed, commandPrefix) {
		return "", false
	}
	rest := trimmed[len(commandPrefix):]
	if rest != "" {
		switch rest[0] {
		case ' ', '\t', '\n', '\r':
		default:
			return "", false
		}
	}
	return strings.TrimSpace(rest), true
}
