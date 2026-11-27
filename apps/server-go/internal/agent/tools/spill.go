// Tool-output spilling on top of the shared helpers in internal/utils: keep
// the useful side of oversized output, save the rest to a file, and tell the
// model the path.
package tools

// countLines returns 1 + the number of newline characters.
func countLines(s string) int {
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}
