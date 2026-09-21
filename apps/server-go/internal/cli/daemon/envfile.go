// Per-machine secret env file (~/.console/env). Port of the env-file
// helpers in apps/cli/daemon-manager.ts: KEY=VALUE lines, quotes optional,
// comments/blank lines ignored. Managed by `console env`, mode 0600.
package daemon

import (
	"os"
	"strings"
)

// ParseEnvFile parses KEY=VALUE lines (quoted values unwrapped); comments
// (#) and blank lines are skipped.
func ParseEnvFile(contents string) map[string]string {
	out := make(map[string]string)
	for _, rawLine := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if len(value) >= 2 {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}
		if key != "" {
			out[key] = value
		}
	}
	return out
}

// LoadEnvFile reads and parses EnvFilePath; returns an empty map (not an
// error) when the file doesn't exist.
func LoadEnvFile() map[string]string {
	data, err := os.ReadFile(EnvFilePath())
	if err != nil {
		return map[string]string{}
	}
	return ParseEnvFile(string(data))
}

// UpsertEnvValues inserts or replaces entries in the env file, leaving
// every other line (comments, blank lines, other keys) byte-identical.
// Creates the file at mode 0600.
func UpsertEnvValues(values map[string]string) error {
	if err := EnsureConsoleDir(); err != nil {
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
