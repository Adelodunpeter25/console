// Tool-output spilling on top of the shared helpers in internal/utils: keep
// the useful side of oversized output, save the rest to a file, and tell the
// model the path.
package tools

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// spilledSection renders one output section that exceeds keepBytes: a
// summary line with the spill path, then the tail of the output. Output
// within the limit passes through unchanged. On a spill error it falls back
// to head truncation rather than losing everything.
func spilledSection(sessionID, base, ext, label, output string, keepBytes int, truncate func(string, int, string) string) string {
	if len(output) <= keepBytes {
		return output
	}
	path, err := utils.SpillFile(utils.SpillDir(sessionID), base, ext, output)
	if err != nil {
		return truncate(output, keepBytes, label)
	}
	lines := countLines(output)
	return fmt.Sprintf("%s: %s, %d lines, full output saved to %s\n--- last %s ---\n%s",
		label, utils.HumanBytes(len(output)), lines, path, utils.HumanBytes(keepBytes), utils.TailOutput(output, keepBytes))
}

func countLines(s string) int {
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			n++
		}
	}
	return n
}
