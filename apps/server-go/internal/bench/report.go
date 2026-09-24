package bench

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

// Summary is the aggregate view of a set of runs.
type Summary struct {
	Runs           int     `json:"runs"`
	SuccessRate    float64 `json:"successRate"`
	MedianTurns    float64 `json:"medianTurns"`
	MedianCostOK   float64 `json:"medianCostPerSuccess"`
	CacheReadShare float64 `json:"cacheReadShare"`
	ColdMissRate   float64 `json:"coldMissRate"`
	ToolErrorRate  float64 `json:"toolErrorRate"`
}

// Summarize aggregates runs (all tasks, or one task's runs).
func Summarize(runs []RunResult) Summary {
	var s Summary
	s.Runs = len(runs)
	if s.Runs == 0 {
		return s
	}
	var turns, okCosts []float64
	successes, input, cacheRead, later, cold, calls, errs := 0, 0, 0, 0, 0, 0, 0
	for _, r := range runs {
		total := r.Usage.Total()
		turns = append(turns, float64(total.Turns))
		if r.Success {
			successes++
			okCosts = append(okCosts, r.CostUSD)
		}
		input += total.Input + total.CacheRead + total.CacheWrite
		cacheRead += total.CacheRead
		if total.Turns > 1 {
			later += total.Turns - 1
		}
		cold += total.ColdMisses
		for _, t := range r.Tools {
			calls += t.Calls
			errs += t.Errors
		}
	}
	s.SuccessRate = float64(successes) / float64(s.Runs)
	s.MedianTurns = median(turns)
	s.MedianCostOK = median(okCosts)
	s.CacheReadShare = ratio(cacheRead, input)
	s.ColdMissRate = ratio(cold, later)
	s.ToolErrorRate = ratio(errs, calls)
	return s
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// byTask groups runs by task id in first-seen order.
func byTask(runs []RunResult) ([]string, map[string][]RunResult) {
	var order []string
	groups := map[string][]RunResult{}
	for _, r := range runs {
		if _, ok := groups[r.Task]; !ok {
			order = append(order, r.Task)
		}
		groups[r.Task] = append(groups[r.Task], r)
	}
	return order, groups
}

// PrintReport writes per-task and overall tables plus estimated token
// share by source.
func PrintReport(w io.Writer, res *Results) {
	fmt.Fprintf(w, "provider=%s model=%s commit=%.12s", res.Provider, res.Model, res.Commit)
	if res.Aborted != "" {
		fmt.Fprintf(w, " ABORTED=%s", res.Aborted)
	}
	if !res.PriceKnown {
		fmt.Fprint(w, " (no price table for model: costs are 0)")
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "task\truns\tsuccess\tturns\t$/success\tcacheRead\tcoldMiss\ttoolErr")
	order, groups := byTask(res.Runs)
	row := func(name string, s Summary) {
		fmt.Fprintf(tw, "%s\t%d\t%.0f%%\t%.1f\t%.4f\t%.0f%%\t%.0f%%\t%.0f%%\n", name, s.Runs,
			s.SuccessRate*100, s.MedianTurns, s.MedianCostOK, s.CacheReadShare*100, s.ColdMissRate*100, s.ToolErrorRate*100)
	}
	for _, id := range order {
		row(id, Summarize(groups[id]))
	}
	row("OVERALL", Summarize(res.Runs))
	tw.Flush()

	sources := map[string]int{}
	total := 0
	for _, r := range res.Runs {
		for k, v := range r.Sources {
			sources[k] += v
			total += v
		}
	}
	if total == 0 {
		return
	}
	keys := make([]string, 0, len(sources))
	for k := range sources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return sources[keys[i]] > sources[keys[j]] })
	fmt.Fprintln(w, "\nestimated input tokens by source (summed over every request):")
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%d\t%.1f%%\n", k, sources[k], float64(sources[k])*100/float64(total))
	}
	tw.Flush()
}

// PrintCompare prints the overall and per-task deltas between two runs.
func PrintCompare(w io.Writer, a, b *Results) {
	fmt.Fprintf(w, "A: %s %s %v\nB: %s %s %v\n", a.Model, strings.TrimSpace(fmt.Sprint(a.Flags)), a.Started.Format("2006-01-02 15:04"),
		b.Model, strings.TrimSpace(fmt.Sprint(b.Flags)), b.Started.Format("2006-01-02 15:04"))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "task\tsuccess A→B\tturns A→B\t$/success A→B\tΔcost\tcacheRead A→B")
	row := func(name string, sa, sb Summary) {
		delta := "n/a"
		if sa.MedianCostOK > 0 {
			delta = fmt.Sprintf("%+.0f%%", (sb.MedianCostOK-sa.MedianCostOK)*100/sa.MedianCostOK)
		}
		fmt.Fprintf(tw, "%s\t%.0f%%→%.0f%%\t%.1f→%.1f\t%.4f→%.4f\t%s\t%.0f%%→%.0f%%\n", name,
			sa.SuccessRate*100, sb.SuccessRate*100, sa.MedianTurns, sb.MedianTurns,
			sa.MedianCostOK, sb.MedianCostOK, delta, sa.CacheReadShare*100, sb.CacheReadShare*100)
	}
	orderA, groupsA := byTask(a.Runs)
	_, groupsB := byTask(b.Runs)
	for _, id := range orderA {
		if runsB, ok := groupsB[id]; ok {
			row(id, Summarize(groupsA[id]), Summarize(runsB))
		}
	}
	row("OVERALL", Summarize(a.Runs), Summarize(b.Runs))
	tw.Flush()
}

// WriteResults saves results as indented JSON.
func WriteResults(path string, res *Results) error {
	raw, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// ReadResults loads a results file.
func ReadResults(path string) (*Results, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var res Results
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &res, nil
}

// EstimateCost projects tasks×runs × the median $/task of a previous run.
func EstimateCost(prev *Results, tasks, runs int) float64 {
	if prev == nil {
		return 0
	}
	var costs []float64
	for _, r := range prev.Runs {
		costs = append(costs, r.CostUSD)
	}
	return median(costs) * float64(tasks*runs)
}
