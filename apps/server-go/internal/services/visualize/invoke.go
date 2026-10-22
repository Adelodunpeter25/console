// The /visualize chat command: detection and task splitting.
//
// Typing "/visualize <task>" activates the visual tool for the session. The
// model has no prior knowledge of it — no tool, no skill listing — so the
// command is matched here rather than discovered by the model. Matching is
// deliberately narrow: the command must open the message (after whitespace),
// followed by whitespace or the end of the message, so "/visualizer" and inline
// mentions never trigger.
package visualize

import "strings"

const commandPrefix = "/visualize"

// SplitInvocation splits a "/visualize <task>" message into its task. It
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
