// Package bench measures the agent harness's cost per finished task:
// it runs a fixed task set against a pinned worktree of this repo,
// checks each result automatically, and records usage, cost, and tool
// stats (docs/plan/harness-token-efficiency.md, Task 1.5).
package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Task is one benchmark task loaded from bench/tasks/*.toml.
type Task struct {
	ID     string `toml:"-"`
	Kind   string `toml:"kind"`
	Prompt string `toml:"prompt"`
	// Setup runs in the fresh worktree before the agent (e.g. break code).
	Setup string `toml:"setup"`
	// Check runs after the agent; exit 0 means success.
	Check string `toml:"check"`
}

// LoadTasks reads every *.toml in dir (sorted by file name). When only is
// non-empty, just those task ids are returned.
func LoadTasks(dir string, only []string) ([]Task, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	want := map[string]bool{}
	for _, id := range only {
		if id = strings.TrimSpace(id); id != "" {
			want[id] = true
		}
	}
	var tasks []Task
	for _, path := range paths {
		id := strings.TrimSuffix(filepath.Base(path), ".toml")
		if len(want) > 0 && !want[id] {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var task Task
		if err := toml.Unmarshal(raw, &task); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if strings.TrimSpace(task.Prompt) == "" || strings.TrimSpace(task.Check) == "" {
			return nil, fmt.Errorf("%s: prompt and check are required", path)
		}
		task.ID = id
		tasks = append(tasks, task)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks found in %s", dir)
	}
	return tasks, nil
}
