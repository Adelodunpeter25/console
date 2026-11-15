// Per-machine secret env file (~/.console/env): KEY=VALUE lines, quotes optional,
// comments/blank lines ignored. Managed by `console env`, mode 0600.
//
// The file itself lives in internal/utils, which owns the storage layout —
// the server reads the same file at boot, so the two agree on where it is.
package daemon

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ParseEnvFile parses KEY=VALUE lines (quoted values unwrapped); comments
// (#) and blank lines are skipped.
func ParseEnvFile(contents string) map[string]string {
	return utils.ParseEnvFile(contents)
}

// LoadEnvFile reads and parses EnvFilePath; returns an empty map (not an
// error) when the file doesn't exist.
func LoadEnvFile() map[string]string {
	return utils.LoadEnvFile()
}

// EnvFilePath is the per-machine secret env file (KEY=VALUE lines) merged
// into the daemon environment on start.
func EnvFilePath() string {
	return utils.EnvFilePath()
}

// UpsertEnvValues inserts or replaces entries in the env file, leaving
// every other line (comments, blank lines, other keys) identical.
// Creates the file at mode 0600.
func UpsertEnvValues(values map[string]string) error {
	if err := EnsureConsoleDir(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(EnvFilePath()), 0o700); err != nil {
		return err
	}
	raw := ""
	if data, err := os.ReadFile(EnvFilePath()); err == nil {
		raw = string(data)
	}
	pending := make(map[string]string, len(values))
	for k, v := range values {
		pending[k] = v
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		eq := strings.Index(trimmed, "=")
		key := ""
		if eq > 0 {
			key = strings.TrimSpace(trimmed[:eq])
		}
		if key != "" {
			if v, ok := pending[key]; ok {
				out = append(out, key+`="`+v+`"`)
				delete(pending, key)
				continue
			}
		}
		out = append(out, line)
	}
	for k, v := range values {
		if _, stillPending := pending[k]; stillPending {
			out = append(out, k+`="`+v+`"`)
		}
	}
	text := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	return os.WriteFile(EnvFilePath(), []byte(text), 0o600)
}
