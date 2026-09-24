// Request dumps for token-efficiency debugging: when CONSOLE_DUMP_REQUESTS
// is set to a directory, every provider request body is written there as
// pretty JSON (<dir>/<conversation>/<seq>-<provider>.json). Off by default.
package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DumpRequestsEnv names the env var holding the dump directory.
const DumpRequestsEnv = "CONSOLE_DUMP_REQUESTS"

var (
	dumpMu  sync.Mutex
	dumpSeq = map[string]int{}
)

// DumpRequest writes one serialized request body when dumping is enabled.
// Failures are logged and never affect the request.
func DumpRequest(provider, conversationID string, body []byte) {
	dir := os.Getenv(DumpRequestsEnv)
	if dir == "" {
		return
	}
	run := sanitizeDumpName(conversationID)
	if run == "" {
		run = "no-conversation"
	}
	dumpMu.Lock()
	dumpSeq[run]++
	seq := dumpSeq[run]
	dumpMu.Unlock()

	target := filepath.Join(dir, run)
	if err := os.MkdirAll(target, 0o755); err != nil {
		slog.Warn("request dump mkdir failed", "dir", target, "error", err)
		return
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(body)
	}
	path := filepath.Join(target, fmt.Sprintf("%04d-%s.json", seq, sanitizeDumpName(provider)))
	if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil {
		slog.Warn("request dump write failed", "path", path, "error", err)
	}
}

func sanitizeDumpName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
}
