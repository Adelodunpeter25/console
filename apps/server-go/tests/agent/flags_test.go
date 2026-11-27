// Coverage for the harness flags package.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/flags"
)

func TestFlagsFromEnv(t *testing.T) {
	env := map[string]string{
		"CONSOLE_HARNESS_SPARSE_LINE_NUMBERS": "1",
		"CONSOLE_HARNESS_UNKNOWN":             "1",
	}
	get := func(key string) string { return env[key] }

	f := flags.FromEnv(get)
	if !f.SparseLineNumbers {
		t.Fatal("SparseLineNumbers should be on")
	}
	// Unknown CONSOLE_HARNESS_ vars are simply not fields yet; FromEnv must
	// not fail on them.
	if f.SparseLineNumbers != flags.FromEnv(get).SparseLineNumbers {
		t.Fatal("FromEnv must be deterministic")
	}

	off := flags.FromEnv(func(string) string { return "" })
	if off.SparseLineNumbers {
		t.Fatal("SparseLineNumbers defaults to off")
	}
}
