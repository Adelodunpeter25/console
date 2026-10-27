package console

import "strings"

// SplitInvocation recognizes the /console activation command. The command is
// intentionally kept out of the persisted message; activation is session state
// and the console-control skill is added to the turn setup instead.
func SplitInvocation(text string) (remainder string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "/console" {
		return "", true
	}
	if strings.HasPrefix(trimmed, "/console ") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "/console ")), true
	}
	return text, false
}
