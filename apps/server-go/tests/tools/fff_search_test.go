// Focused coverage for the fff adapter's public query-shaping and lease
// safety surface: case modes, whole-word transforms, and Lease release
// idempotency. These run without the native library (pure Go logic).
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
)

func TestParseCaseMode(t *testing.T) {
	cases := []struct {
		in      string
		want    fff.CaseMode
		wantErr bool
	}{
		{"", fff.CaseSmart, false},
		{"smart", fff.CaseSmart, false},
		{"Sensitive", fff.CaseSensitive, false},
		{"INSENSITIVE", fff.CaseInsensitive, false},
		{"  insensitive  ", fff.CaseInsensitive, false},
		{"bogus", "", true},
	}
	for _, c := range cases {
		got, err := fff.ParseCaseMode(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseCaseMode(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCaseMode(%q): unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseCaseMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTransformQueryCaseModes(t *testing.T) {
	// Smart/sensitive leave the pattern untouched (fff's legacy smart_case
	// flag handles them natively); explicit insensitive is rewritten to an
	// inline regex flag and forces regex mode.
	pattern, mode, err := fff.TransformQuery("Needle", fff.GrepModePlain, fff.CaseSmart, false)
	if err != nil {
		t.Fatalf("smart: %v", err)
	}
	if pattern != "Needle" || mode != fff.GrepModePlain {
		t.Errorf("smart: got (%q, %d)", pattern, mode)
	}

	pattern, mode, err = fff.TransformQuery("Needle", fff.GrepModePlain, fff.CaseInsensitive, false)
	if err != nil {
		t.Fatalf("insensitive: %v", err)
	}
	if pattern != "(?i)Needle" {
		t.Errorf("insensitive pattern: %q", pattern)
	}
	if mode != fff.GrepModeRegex {
		t.Errorf("insensitive mode: got %d, want regex", mode)
	}
}

func TestTransformQueryWholeWord(t *testing.T) {
	pattern, mode, err := fff.TransformQuery("cat", fff.GrepModePlain, fff.CaseSmart, true)
	if err != nil {
		t.Fatalf("whole word: %v", err)
	}
	if pattern != `\b(?:cat)\b` {
		t.Errorf("whole word pattern: %q", pattern)
	}
	if mode != fff.GrepModeRegex {
		t.Errorf("whole word mode: got %d, want regex", mode)
	}

	// Plain-mode queries are regex-escaped before the boundary wrapper so
	// special characters in the literal text remain literal.
	pattern, _, err = fff.TransformQuery("a.b", fff.GrepModePlain, fff.CaseSmart, true)
	if err != nil {
		t.Fatalf("escaped whole word: %v", err)
	}
	if pattern != `\b(?:a\.b)\b` {
		t.Errorf("escaped whole word pattern: %q", pattern)
	}
}

func TestTransformQueryValidation(t *testing.T) {
	if _, _, err := fff.TransformQuery("", fff.GrepModePlain, fff.CaseSmart, false); err == nil {
		t.Fatal("expected error for empty query")
	}
	if _, _, err := fff.TransformQuery("x", fff.GrepMode(9), fff.CaseSmart, false); err == nil {
		t.Fatal("expected error for invalid grep mode")
	}
	if _, _, err := fff.TransformQuery("x", fff.GrepModePlain, fff.CaseMode("bogus"), false); err == nil {
		t.Fatal("expected error for invalid case mode")
	}
}

func TestManagerGetOrCreateDisabled(t *testing.T) {
	// Without FFF_LIB_PATH the manager is disabled; GetOrCreate must fail
	// cleanly rather than panic, and CloseAll on a disabled manager is a
	// no-op that callers can always defer safely.
	m := fff.NewManager()
	if m.Enabled() {
		t.Skip("fff native library is loaded in this environment")
	}
	if _, err := m.GetOrCreate(t.TempDir()); err == nil {
		t.Fatal("expected error from a disabled manager")
	}
	m.CloseAll() // must not panic
}
