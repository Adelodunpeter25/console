// Bench runner: fake-provider runs against a scratch git repo (no
// network), success checks, answer capture, report output, and the usage
// budget guard.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/bench"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func scratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=b@b", "-c", "user.name=b", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

// writerProvider writes hello.txt via write_file, then answers "done 42".
func writerProvider() *helpers.MockProvider {
	args, _ := json.Marshal(map[string]string{"path": "hello.txt", "content": "hi"})
	call := tools.ToolCall{ID: "w1", Name: "write_file", Arguments: args}
	return &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventToolCall, Call: &call},
				{Kind: loop.EventUsage, Usage: &loop.TurnUsage{Input: 1000, CacheWrite: 2000, Output: 100}}}
		},
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: "done 42"},
				{Kind: loop.EventUsage, Usage: &loop.TurnUsage{Input: 50, CacheRead: 2000, Output: 20}}}
		},
	}}
}

func TestBenchRunChecksSuccess(t *testing.T) {
	repo := scratchRepo(t)
	tasks := []bench.Task{
		{ID: "write", Prompt: "write hello", Check: "test -f hello.txt"},
		{ID: "answer", Prompt: "answer", Check: `grep -q 42 "$BENCH_ANSWER"`},
		{ID: "fails", Prompt: "nope", Check: "test -f missing.txt"},
	}
	var results []*bench.Results
	for _, task := range tasks {
		res, err := bench.Run(context.Background(), bench.Options{
			Provider: writerProvider(), ProviderID: "mock", Model: "claude-haiku-4-5",
			Tasks: []bench.Task{task}, Runs: 1, Repo: repo, Log: &bytes.Buffer{},
		})
		if err != nil {
			t.Fatalf("%s: %v", task.ID, err)
		}
		results = append(results, res)
	}
	for i, want := range []bool{true, true, false} {
		if got := results[i].Runs[0]; got.Success != want {
			t.Fatalf("%s: success=%v want %v (err=%q check=%q)", tasks[i].ID, got.Success, want, got.Error, got.CheckOut)
		}
	}
	run := results[0].Runs[0]
	if run.Usage.Turns != 2 || run.Usage.CacheRead != 2000 || run.Usage.ColdMisses != 0 {
		t.Fatalf("usage: %+v", run.Usage)
	}
	// (1000*1 + 2000*1.25 + 50*1 + 2000*0.1 + 120*5) / 1e6
	if want := 0.00435; run.CostUSD < want-1e-9 || run.CostUSD > want+1e-9 {
		t.Fatalf("cost: %v", run.CostUSD)
	}
	if run.Tools["write_file"].Calls != 1 || run.Sources["tools"] == 0 || run.Sources["system:identity"] == 0 {
		t.Fatalf("tools/sources: %+v %+v", run.Tools, run.Sources)
	}
	// Worktrees never leak into the source repo.
	if _, err := os.Stat(filepath.Join(repo, "hello.txt")); err == nil {
		t.Fatal("agent wrote into the source repo")
	}
}

func TestBenchReportAndResultsRoundTrip(t *testing.T) {
	repo := scratchRepo(t)
	res, err := bench.Run(context.Background(), bench.Options{
		Provider: writerProvider(), ProviderID: "mock", Model: "claude-haiku-4-5",
		Tasks: []bench.Task{{ID: "write", Prompt: "p", Check: "true"}}, Runs: 2, Repo: repo, Log: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "r.json")
	if err := bench.WriteResults(path, res); err != nil {
		t.Fatal(err)
	}
	back, err := bench.ReadResults(path)
	if err != nil || len(back.Runs) != 2 {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	var out bytes.Buffer
	bench.PrintReport(&out, back)
	bench.PrintCompare(&out, res, back)
	for _, want := range []string{"OVERALL", "100%", "estimated input tokens by source", "+0%"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report missing %q:\n%s", want, out.String())
		}
	}
	if got := bench.EstimateCost(back, 3, 2); got <= 0 {
		t.Fatalf("estimate: %v", got)
	}
}

func TestLoadTasks(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.toml"), []byte("prompt = \"p\"\ncheck = \"true\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.toml"), []byte("prompt = \"q\"\ncheck = \"true\"\n"), 0o644)
	tasks, err := bench.LoadTasks(dir, []string{"b"})
	if err != nil || len(tasks) != 1 || tasks[0].ID != "b" {
		t.Fatalf("filter: %v %+v", err, tasks)
	}
	os.WriteFile(filepath.Join(dir, "c.toml"), []byte("prompt = \"no check\"\n"), 0o644)
	if _, err := bench.LoadTasks(dir, nil); err == nil {
		t.Fatal("task without check must fail")
	}
}

func TestShippedTasksParse(t *testing.T) {
	tasks, err := bench.LoadTasks("../../bench/tasks", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 12 {
		t.Fatalf("want 12 tasks, got %d", len(tasks))
	}
}

// fakeUsage returns scripted reports, one per Fetch. A negative percent
// returns a nil report (the usage service's fetch-failure shape).
type fakeUsage struct {
	used  []float64
	calls int
}

func (f *fakeUsage) Fetch(ctx context.Context) (*usage.Report, error) {
	pct := f.used[min(f.calls, len(f.used)-1)]
	f.calls++
	if pct < 0 {
		return nil, nil
	}
	return &usage.Report{Limits: []usage.Limit{
		{Label: "5-hour", Amount: usage.BuildPercentAmount(&pct)},
		{Label: "credits", Amount: usage.Amount{Unit: "usd"}},
	}}, nil
}

func TestBudgetRefusesAboveThreshold(t *testing.T) {
	budget := &bench.Budget{Source: &fakeUsage{used: []float64{85}}, MaxPct: 80}
	if _, err := budget.Headroom(context.Background()); err == nil || !strings.Contains(err.Error(), "5-hour") {
		t.Fatalf("want refusal naming the limit, got %v", err)
	}
	ok := &bench.Budget{Source: &fakeUsage{used: []float64{40}}, MaxPct: 80}
	if used, err := ok.Headroom(context.Background()); err != nil || used != 40 {
		t.Fatalf("under threshold: %v %v", used, err)
	}
}

func TestBudgetStopsMidRunWithPartialResults(t *testing.T) {
	repo := scratchRepo(t)
	budget := &bench.Budget{Source: &fakeUsage{used: []float64{10, 90}}, MaxPct: 80, MinInterval: time.Nanosecond}
	res, err := bench.Run(context.Background(), bench.Options{
		Provider: writerProvider(), ProviderID: "mock", Model: "claude-haiku-4-5",
		Tasks: []bench.Task{{ID: "a", Prompt: "p", Check: "true"}, {ID: "b", Prompt: "p", Check: "true"}},
		Runs:  1, Repo: repo, Budget: budget, Log: &bytes.Buffer{},
	})
	if !errors.Is(err, bench.ErrBudget) {
		t.Fatalf("want budget stop, got %v", err)
	}
	if res == nil || res.Aborted != "budget" || len(res.Runs) != 1 || res.Runs[0].Task != "a" {
		t.Fatalf("partial results: %+v", res)
	}
	path := filepath.Join(t.TempDir(), "partial.json")
	if err := bench.WriteResults(path, res); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"aborted": "budget"`) {
		t.Fatalf("partial file: %s", raw)
	}
}

func TestBudgetEmptyReport(t *testing.T) {
	retried := &bench.Budget{Source: &fakeUsage{used: []float64{-1, 30}}, MaxPct: 80, RetryDelay: time.Millisecond}
	if used, err := retried.Headroom(context.Background()); err != nil || used != 30 {
		t.Fatalf("retry after empty report: %v %v", used, err)
	}
	failing := &bench.Budget{Source: &fakeUsage{used: []float64{-1}}, MaxPct: 80, RetryDelay: time.Millisecond}
	if _, err := failing.Headroom(context.Background()); err == nil {
		t.Fatal("repeated empty report must stop the bench")
	}
}

func TestBudgetThrottlesAndReusesLastGoodReport(t *testing.T) {
	source := &fakeUsage{used: []float64{20, 99}}
	cached := &bench.Budget{Source: source, MaxPct: 80}
	for i := 0; i < 3; i++ {
		if used, err := cached.Headroom(context.Background()); err != nil || used != 20 {
			t.Fatalf("check %d: %v %v", i, used, err)
		}
	}
	if source.calls != 1 {
		t.Fatalf("fetches within MinInterval: %d", source.calls)
	}
	// Rate-limited (empty) fetches fall back to the last good report.
	stale := &bench.Budget{Source: &fakeUsage{used: []float64{20, -1}}, MaxPct: 80,
		MinInterval: time.Nanosecond, RetryDelay: time.Millisecond}
	stale.Headroom(context.Background())
	if used, err := stale.Headroom(context.Background()); err != nil || used != 20 {
		t.Fatalf("stale fallback: %v %v", used, err)
	}
}

func TestBenchOnProgressAfterEveryRun(t *testing.T) {
	repo := scratchRepo(t)
	var seen []int
	_, err := bench.Run(context.Background(), bench.Options{
		Provider: writerProvider(), ProviderID: "mock", Model: "claude-haiku-4-5",
		Tasks: []bench.Task{{ID: "a", Prompt: "p", Check: "true"}}, Runs: 2, Repo: repo, Log: &bytes.Buffer{},
		OnProgress: func(r *bench.Results) { seen = append(seen, len(r.Runs)) },
	})
	if err != nil || len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("progress: %v %v", err, seen)
	}
}
