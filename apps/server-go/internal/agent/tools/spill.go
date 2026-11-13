// Spilling oversized tool output to files so truncation never destroys data.
//
// The shape is always: keep the useful side of the output (the tail for
// command output, where errors print; the head for fetched pages), save the
// full content to a file, and tell the model the path so it can read or grep
// the rest.
package tools

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// spillRoot is where oversized tool output is saved. Under the system temp
// dir: the files are a recovery aid for the current run, not durable state,
// and the OS clears the directory on its own schedule.
func spillRoot() string {
	return filepath.Join(os.TempDir(), "console-tool-output")
}

var spillNameRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// spillDir namespaces spill files per session. An empty session id falls
// back to the shared root (package-level tools have no session).
func spillDir(sessionID string) string {
	id := strings.Trim(spillNameRe.ReplaceAllString(sessionID, "-"), "-")
	if id == "" {
		return spillRoot()
	}
	return filepath.Join(spillRoot(), id)
}

// spillToFile writes content to dir under a name built from base, a
// nanosecond suffix and ext, and returns the full path.
func spillToFile(dir, base, ext, content string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// spillURLName derives a stable file base name from a URL so re-fetches of
// the same page overwrite rather than accumulate.
func spillURLName(url string) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s", url)
	return fmt.Sprintf("fetch-%016x", h.Sum64())
}

// tailOutput keeps the last maxBytes bytes of output, starting on a whole
// line. Errors and summaries print at the end of command output; that is the
// side worth keeping.
func tailOutput(output string, maxBytes int) string {
	if len(output) <= maxBytes {
		return output
	}
	tail := output[len(output)-maxBytes:]
	if i := strings.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	return tail
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// spilledSection renders one output section that exceeds keepBytes: a
// summary line with the spill path, then the tail of the output. Output
// within the limit passes through unchanged. On a spill error it falls back
// to head truncation rather than losing everything.
func spilledSection(dir, base, ext, label, output string, keepBytes int) string {
	if len(output) <= keepBytes {
		return output
	}
	path, err := spillToFile(dir, base, ext, output)
	if err != nil {
		return truncateOutput(output, keepBytes, label)
	}
	lines := strings.Count(output, "\n") + 1
	return fmt.Sprintf("%s: %s, %d lines, full output saved to %s\n--- last %s ---\n%s",
		label, humanBytes(len(output)), lines, path, humanBytes(keepBytes), tailOutput(output, keepBytes))
}
