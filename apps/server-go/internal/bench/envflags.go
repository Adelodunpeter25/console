package bench

import (
	"strings"
)

// HarnessEnvPrefix marks environment variables that toggle harness features.
const HarnessEnvPrefix = "CONSOLE_HARNESS_"

// AddEnvFlags records every set CONSOLE_HARNESS_* variable from environ
// into flags (unless already given via --flags), so results show which
// env-var features were on.
func AddEnvFlags(flags map[string]string, environ []string) {
	for _, kv := range environ {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, HarnessEnvPrefix) || value == "" {
			continue
		}
		if _, exists := flags[key]; !exists {
			flags[key] = value
		}
	}
}
