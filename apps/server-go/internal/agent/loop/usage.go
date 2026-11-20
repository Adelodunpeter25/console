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
	// MissTurns counts turns whose prompt was large but mostly not served from
	// cache (see IsCacheMiss). Unlike ColdMisses it catches partial rewrites,
	// such as a turn that read 695 tokens and wrote 64k.
	MissTurns int `json:"missTurns"`
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
	u.MissTurns += o.MissTurns
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
	if IsCacheMiss(turn) {
		u.MissTurns++
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

// ContextSnapshot is the session's context-window occupancy at a point in
// time: estimated payload tokens against the run model's window. Estimated
// with the same heuristic the compaction threshold uses, so the footer ring
// and the autocompact trigger always agree.
type ContextSnapshot struct {
	UsedTokens     int     `json:"usedTokens"`
	ContextWindow  int     `json:"contextWindow"`
	PercentUsed    float64 `json:"percentUsed"`
	ThresholdRatio float64 `json:"thresholdRatio"`
	ModelID        string  `json:"modelId"`
	Provider       string  `json:"provider"`
	Source         string  `json:"source"`
}

// Cache-miss detection: a prompt this large should mostly come from cache
// after the first request; below it the numbers are too noisy to judge.
const (
	cacheMissMinPromptTokens = 2000
	cacheMissHitThreshold    = 0.5
)

// PromptTokens is everything the model read this turn: fresh input, cache
// reads and cache writes.
func (t TurnUsage) PromptTokens() int { return t.Input + t.CacheRead + t.CacheWrite }

// HitRatio is the share of the prompt served from cache, or -1 when empty.
func (t TurnUsage) HitRatio() float64 {
	total := t.PromptTokens()
	if total == 0 {
		return -1
	}
	return float64(t.CacheRead) / float64(total)
}

// IsCacheMiss reports a turn with a large prompt that was mostly not read
// from cache, whether it read nothing or only a small fixed prefix.
func IsCacheMiss(t *TurnUsage) bool {
	if t == nil || t.PromptTokens() < cacheMissMinPromptTokens {
		return false
	}
	return t.HitRatio() < cacheMissHitThreshold
}
