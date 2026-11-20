// Cache-miss detection catches partial rewrites that the zero-read
// ColdMisses counter misses (a turn that read 695 tokens and wrote 64k).
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

func TestIsCacheMiss(t *testing.T) {
	cases := []struct {
		name  string
		usage *loop.TurnUsage
		want  bool
	}{
		{"nil", nil, false},
		{"healthy", &loop.TurnUsage{Input: 500, CacheRead: 60000, CacheWrite: 1200}, false},
		{"full miss", &loop.TurnUsage{Input: 50000}, true},
		{"claude partial rewrite", &loop.TurnUsage{CacheRead: 695, CacheWrite: 64069}, true},
		{"small prompt is too noisy to judge", &loop.TurnUsage{Input: 900}, false},
	}
	for _, c := range cases {
		if got := loop.IsCacheMiss(c.usage); got != c.want {
			t.Errorf("%s: IsCacheMiss = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRunUsageCountsMissTurnsThatColdMissesCannotSee(t *testing.T) {
	var tracker loop.UsageTracker
	tracker.AddTurn(&loop.TurnUsage{Input: 4, CacheRead: 63000, CacheWrite: 900})
	tracker.AddTurn(&loop.TurnUsage{CacheRead: 695, CacheWrite: 64069})
	got := tracker.Snapshot()
	if got.ColdMisses != 0 {
		t.Fatalf("a partial rewrite still read some cache, so ColdMisses stays 0, got %d", got.ColdMisses)
	}
	if got.MissTurns != 1 {
		t.Fatalf("MissTurns = %d, want 1", got.MissTurns)
	}
}
