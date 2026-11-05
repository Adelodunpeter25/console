// The /browser-use chat command: detection and task splitting, mirroring
// /computer-use.
//
// Matching is deliberately narrow: the command must open the message (after
// whitespace), followed by whitespace or the end of the message, so
// "/browser-useful" and inline mentions never trigger.
package browseruse

import "strings"

// commandPrefix is the leading command that activates browser use.
const commandPrefix = "/browser-use"

// SplitInvocation splits a "/browser-use <task>" message into its task. It
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
