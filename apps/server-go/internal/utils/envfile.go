// Per-machine env file (<storage>/env): KEY=VALUE lines, quotes optional,
// comments and blank lines ignored. `console env` writes it; the server
// loads it at boot so a harness flag set there reaches every start path.
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvFilePath is the per-machine env file inside the storage directory.
func EnvFilePath() string {
	return filepath.Join(ConsoleStorageDir(), "env")
}

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

// ApplyEnvFile puts every entry of the env file into the process
// environment. A variable already set in the environment wins: the file
// fills gaps, it never overrides an explicit setting.
func ApplyEnvFile() {
	for key, value := range LoadEnvFile() {
		if _, ok := os.LookupEnv(key); ok {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
