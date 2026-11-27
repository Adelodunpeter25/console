package routes

import "testing"

func TestParseMediaRange(t *testing.T) {
	cases := []struct {
		header string
		total  int64
		start  int64
		end    int64
		ok     bool
	}{
		{"bytes=0-9", 100, 0, 9, true},
		{"bytes=90-", 100, 90, 99, true},
		{"bytes=-10", 100, 90, 99, true},    // suffix: last 10 bytes
		{"bytes=-500", 100, 0, 99, true},    // suffix larger than file clamps
		{"bytes=50-200", 100, 50, 99, true}, // end clamps to total-1
		{"bytes=100-", 100, 0, 0, false},    // start >= total
		{"bytes=0-0,2-2", 100, 0, 0, false}, // multi-range unsupported
		{"bytes=abc", 100, 0, 0, false},     // malformed
		{"items=0-9", 100, 0, 0, false},     // wrong unit
		{"bytes=10-5", 100, 0, 0, false},    // end < start
		{"bytes=", 100, 0, 0, false},        // empty
		{"bytes=-", 100, 0, 0, false},       // empty suffix
		{"bytes=0-9", 0, 0, 0, false},       // empty file
	}
	for _, tc := range cases {
		start, end, ok := parseMediaRange(tc.header, tc.total)
		if ok != tc.ok || (ok && (start != tc.start || end != tc.end)) {
			t.Errorf("parseMediaRange(%q, %d) = (%d, %d, %v), want (%d, %d, %v)",
				tc.header, tc.total, start, end, ok, tc.start, tc.end, tc.ok)
		}
	}
}
