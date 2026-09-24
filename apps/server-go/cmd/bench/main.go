// Bench: measures agent harness cost per finished task against a fixed
// task set (docs/plan/harness-token-efficiency.md, Phase 1).
//
//	go run ./cmd/bench --runs 2 --out bench/results/baseline.json
//	go run ./cmd/bench compare a.json b.json
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/bench"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "compare" {
		if err := compare(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compare(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: bench compare a.json b.json")
	}
	a, err := bench.ReadResults(args[0])
	if err != nil {
		return err
	}
	b, err := bench.ReadResults(args[1])
	if err != nil {
		return err
	}
	bench.PrintCompare(os.Stdout, a, b)
	return nil
}

func run() error {
	providerID := flag.String("provider", "claude", "provider id")
	model := flag.String("model", "claude-haiku-4-5", "model id")
	runs := flag.Int("runs", 2, "runs per task")
	taskList := flag.String("tasks", "", "comma-separated task ids (default: all)")
	taskDir := flag.String("task-dir", "bench/tasks", "task directory")
	flagList := flag.String("flags", "", "harness feature flags key=value,...")
	maxPct := flag.Float64("max-usage-pct", 0, "stop when any subscription limit reaches this percent (claude only; 0 = off, the default)")
	out := flag.String("out", "", "results JSON path")
	prev := flag.String("prev", "", "previous results file for the cost estimate")
	yes := flag.Bool("yes", false, "skip the cost-estimate confirmation")
	commit := flag.String("commit", "HEAD", "commit to check out for every run")
	flag.Parse()

	tasks, err := bench.LoadTasks(*taskDir, strings.Split(*taskList, ","))
	if err != nil {
		return err
	}
	provider, err := providers.Lookup(*providerID)
	if err != nil {
		return err
	}
	repo, err := repoRoot()
	if err != nil {
		return err
	}
	flags, err := parseFlags(*flagList)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var budget *bench.Budget
	if *providerID == "claude" && *maxPct > 0 {
		budget = &bench.Budget{Source: bench.ServiceSource{Service: usage.NewService(), Provider: "claude"}, MaxPct: *maxPct}
		used, err := budget.Headroom(ctx)
		if err != nil {
			return fmt.Errorf("refusing to start: %w", err)
		}
		fmt.Fprintf(os.Stderr, "claude usage: highest limit at %.0f%% (max %.0f%%)\n", used, *maxPct)
	}
	if *prev != "" {
		prevResults, err := bench.ReadResults(*prev)
		if err != nil {
			return err
		}
		estimate := bench.EstimateCost(prevResults, len(tasks), *runs)
		fmt.Fprintf(os.Stderr, "estimated cost: $%.2f (%d tasks × %d runs)\n", estimate, len(tasks), *runs)
		if estimate > 1 && !*yes {
			return fmt.Errorf("estimate above $1: re-run with --yes to continue")
		}
	}

	save := func(results *bench.Results) {
		if *out == "" {
			return
		}
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err == nil {
			err = bench.WriteResults(*out, results)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "save %s: %v\n", *out, err)
		}
	}
	results, runErr := bench.Run(ctx, bench.Options{
		Provider: provider, ProviderID: *providerID, Model: *model,
		Tasks: tasks, Runs: *runs, Repo: repo, Commit: *commit,
		Flags: flags, Budget: budget, OnProgress: save,
	})
	if results != nil {
		save(results)
		bench.PrintReport(os.Stdout, results)
		if *out != "" {
			fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
		}
	}
	return runErr
}

func parseFlags(list string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(list, ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("bad flag %q (want key=value)", pair)
		}
		out[key] = value
	}
	return out, nil
}

// repoRoot finds the enclosing git repository.
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("not inside a git repository: %s", wd)
		}
	}
}
