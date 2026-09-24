// Run-level token accounting: sums every provider turn's TurnUsage for one
// agent run, with nested subagent runs rolled up separately so the whole
// tree can be measured (docs/plan/harness-token-efficiency.md, Task 1.1).
package loop

import "sync"

// RunUsage is the aggregated token usage of one agent run. Top-level
// fields count only this agent's own turns; Subagents holds the combined
// usage of every nested subagent run.
type RunUsage struct {
	Turns      int `json:"turns"`
	Input      int `json:"input"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
	Output     int `json:"output"`
	Reasoning  int `json:"reasoning"`
	// ColdMisses counts turns after the first that read nothing from cache.
	ColdMisses int `json:"coldMisses"`
	// UnreportedTurns counts turns whose provider sent no usage block.
	UnreportedTurns int `json:"unreportedTurns"`

	Subagents *RunUsage `json:"subagents,omitempty"`
}

func (u *RunUsage) merge(o RunUsage) {
	u.Turns += o.Turns
	u.Input += o.Input
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
	u.Output += o.Output
	u.Reasoning += o.Reasoning
	u.ColdMisses += o.ColdMisses
	u.UnreportedTurns += o.UnreportedTurns
}

// Total returns own turns plus all subagent turns as one flat usage.
func (u RunUsage) Total() RunUsage {
	out := u
	out.Subagents = nil
	if u.Subagents != nil {
		out.merge(u.Subagents.Total())
	}
	return out
}

// UsageTracker accumulates RunUsage for one run. Safe for concurrent use
// (subagents report from tool goroutines).
type UsageTracker struct {
	mu    sync.Mutex
	usage RunUsage
}

// AddTurn records one provider turn. A nil usage counts as unreported.
func (t *UsageTracker) AddTurn(turn *TurnUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	u := &t.usage
	u.Turns++
	if turn == nil {
		u.UnreportedTurns++
		return
	}
	u.Input += turn.Input
	u.CacheRead += turn.CacheRead
	u.CacheWrite += turn.CacheWrite
	u.Output += turn.Output
	if turn.ReasoningTokens != nil {
		u.Reasoning += *turn.ReasoningTokens
	}
	if u.Turns > 1 && turn.CacheRead == 0 {
		u.ColdMisses++
	}
}

// AddSubagent folds a finished subagent run (including its own nested
// subagents) into this run's Subagents total.
func (t *UsageTracker) AddSubagent(child RunUsage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.usage.Subagents == nil {
		t.usage.Subagents = &RunUsage{}
	}
	t.usage.Subagents.merge(child.Total())
}

// Snapshot returns a copy of the current totals.
func (t *UsageTracker) Snapshot() RunUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.usage
	if t.usage.Subagents != nil {
		sub := *t.usage.Subagents
		out.Subagents = &sub
	}
	return out
}
