package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Options configures one bench invocation.
type Options struct {
	Provider   loop.Provider
	ProviderID string
	Model      string
	Tasks      []Task
	Runs       int
	// Repo is the git repo to copy; Commit pins the worktree (default HEAD).
	Repo   string
	Commit string
	// Flags are harness feature flags, recorded with the results.
	Flags map[string]string
	// Budget, when set, is checked before the first run and between runs.
	Budget *Budget
	// Timeout bounds one agent run (default 10 minutes).
	Timeout time.Duration
	// Log receives progress lines (default os.Stderr).
	Log io.Writer
}

// RunResult is one task attempt.
type RunResult struct {
	Task     string                   `json:"task"`
	Run      int                      `json:"run"`
	Success  bool                     `json:"success"`
	Error    string                   `json:"error,omitempty"`
	Usage    loop.RunUsage            `json:"usage"`
	CostUSD  float64                  `json:"costUsd"`
	Tools    map[string]loop.ToolStat `json:"tools"`
	Sources  map[string]int           `json:"sources"`
	WallMs   int64                    `json:"wallMs"`
	CheckOut string                   `json:"checkOutput,omitempty"`
	// Answer is the final assistant text (trimmed), for failed checks.
	Answer string `json:"answer,omitempty"`
}

// Results is a full bench run, written as JSON.
type Results struct {
	Provider   string            `json:"provider"`
	Model      string            `json:"model"`
	Commit     string            `json:"commit"`
	Flags      map[string]string `json:"flags,omitempty"`
	Started    time.Time         `json:"started"`
	Aborted    string            `json:"aborted,omitempty"`
	PriceKnown bool              `json:"priceKnown"`
	Runs       []RunResult       `json:"runs"`
}

// ErrBudget marks a run stopped by the usage guard.
var ErrBudget = errors.New("budget")

// Run executes every task Runs times. Partial results are always returned;
// a budget stop sets Aborted to "budget".
func Run(ctx context.Context, opts Options) (*Results, error) {
	if opts.Runs <= 0 {
		opts.Runs = 1
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	if opts.Log == nil {
		opts.Log = os.Stderr
	}
	commit, err := resolveCommit(opts.Repo, opts.Commit)
	if err != nil {
		return nil, err
	}
	_, known := PriceFor(opts.Model)
	results := &Results{
		Provider: opts.ProviderID, Model: opts.Model, Commit: commit, Flags: opts.Flags,
		Started: time.Now().UTC(), PriceKnown: known,
	}
	for _, task := range opts.Tasks {
		for run := 1; run <= opts.Runs; run++ {
			if used, err := opts.Budget.Headroom(ctx); err != nil {
				fmt.Fprintf(opts.Log, "stopping: %v\n", err)
				results.Aborted = ErrBudget.Error()
				return results, fmt.Errorf("%w: %v", ErrBudget, err)
			} else if opts.Budget != nil {
				fmt.Fprintf(opts.Log, "usage %.0f%%\n", used)
			}
			fmt.Fprintf(opts.Log, "[%s run %d] starting\n", task.ID, run)
			result := runOne(ctx, opts, commit, task, run)
			fmt.Fprintf(opts.Log, "[%s run %d] success=%v turns=%d cost=$%.4f %s\n",
				task.ID, run, result.Success, result.Usage.Total().Turns, result.CostUSD, result.Error)
			results.Runs = append(results.Runs, result)
			if isRateLimited(result.Error) {
				results.Aborted = ErrBudget.Error()
				return results, fmt.Errorf("%w: rate limited after retries", ErrBudget)
			}
			if ctx.Err() != nil {
				results.Aborted = "cancelled"
				return results, ctx.Err()
			}
		}
	}
	return results, nil
}

func isRateLimited(errText string) bool {
	lower := strings.ToLower(errText)
	return strings.Contains(lower, "429") || strings.Contains(lower, "rate limit")
}

func runOne(parent context.Context, opts Options, commit string, task Task, run int) (result RunResult) {
	result = RunResult{Task: task.ID, Run: run, Sources: map[string]int{}}
	start := time.Now()
	defer func() { result.WallMs = time.Since(start).Milliseconds() }()

	dir, cleanup, err := newWorktree(opts.Repo, commit)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer cleanup()

	// File tools resolve relative paths against the process cwd, so each
	// run executes inside its worktree (runs are sequential).
	prevWD, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		result.Error = err.Error()
		return result
	}
	defer os.Chdir(prevWD)

	if task.Setup != "" {
		if out, err := shell(parent, dir, task.Setup); err != nil {
			result.Error = fmt.Sprintf("setup failed: %v: %s", err, tail(out))
			return result
		}
	}

	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	agent, toolDefs := buildAgent(opts, dir, task)
	agent.OnRequest = func(b loop.Breakdown) { addSources(result.Sources, b) }
	events, err := agent.Run(ctx, "bench-"+task.ID, task.Prompt, toolDefs)
	answer := ""
	if err == nil {
		for {
			event, streamErr, ok := events.Next()
			if !ok {
				err = streamErr
				break
			}
			if event.Kind == loop.EventTurnDone {
				answer = finalText(event.Message)
			}
		}
	}
	result.Usage = agent.Usage.Snapshot()
	result.Tools = agent.ToolStats.Snapshot()
	if price, ok := PriceFor(opts.Model); ok {
		result.CostUSD = Cost(price, result.Usage.Total())
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	// Checks see the final answer via $BENCH_ANSWER (question tasks).
	answerPath := filepath.Join(filepath.Dir(dir), "answer.txt")
	if err := os.WriteFile(answerPath, []byte(answer), 0o644); err != nil {
		result.Error = err.Error()
		return result
	}
	out, checkErr := shell(parent, dir, "export BENCH_ANSWER="+answerPath+"\n"+task.Check)
	result.Success = checkErr == nil
	if !result.Success {
		result.CheckOut = tail(out)
		result.Answer = tail(answer)
	}
	return result
}

// buildAgent mirrors the app's run setup (run/turns.go): same system
// prompt builder and default tools, full-access mode, auto-answered asks.
func buildAgent(opts Options, dir string, task Task) (*loop.Agent, []tools.Definition) {
	prompt := systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
		Cwd: dir, Model: opts.Model, ApprovalMode: systemprompt.FullAccess,
	})
	ask := func(ctx context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
		return tools.AskAnswer{Text: "No preference. Use your best judgment and continue."}, nil
	}
	usage := &loop.UsageTracker{}
	var toolList []tools.Tool
	for _, t := range tools.DefaultTools() {
		switch t.Name() {
		case "ask":
			toolList = append(toolList, tools.NewAskTool(ask))
		case "askMany":
			toolList = append(toolList, tools.NewAskManyTool(ask))
		default:
			toolList = append(toolList, t)
		}
	}
	toolList = append(toolList, tools.NewMemoryTool("", nil))
	for i, t := range toolList {
		if t.Name() == "subagent" {
			toolList[i] = loop.NewSubagentTool(&loop.SubagentContext{
				Provider: opts.Provider, Tools: toolList, SystemPrompt: prompt.SystemPrompt, Usage: usage,
			})
		}
	}
	registry := tools.NewRegistry(toolList...)
	agent := loop.New(opts.Provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	agent.Usage = usage
	agent.SystemPrompt = prompt.SystemPrompt
	for _, s := range prompt.Sections {
		agent.SystemSections = append(agent.SystemSections, loop.NamedText{Name: s.Name, Content: s.Content})
	}
	agent.Model = opts.Model
	agent.CacheRetention = loop.CacheShort
	agent.ConversationID = fmt.Sprintf("bench-%s-%d:%s:%s", task.ID, time.Now().UnixNano(), opts.ProviderID, opts.Model)
	return agent, registry.Definitions()
}

// addSources folds one request's estimate into per-source totals, keyed
// "system:<section>", "tools", "history:<bucket>", "results:<tool>".
func addSources(into map[string]int, b loop.Breakdown) {
	for name, v := range b.System {
		into["system:"+name] += v
	}
	for _, v := range b.Tools {
		into["tools"] += v
	}
	into["history:user"] += b.UserText
	into["history:assistant"] += b.AssistantText
	into["history:toolCallArgs"] += b.ToolCallArgs
	into["history:summaries"] += b.Summaries
	for name, v := range b.ToolResults {
		into["results:"+name] += v
	}
}

// finalText joins the text parts of the final assistant message.
func finalText(message any) string {
	assistant, ok := message.(loop.AssistantMessage)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range assistant.Content {
		if text, ok := part.(loop.TextPart); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

func resolveCommit(repo, commit string) (string, error) {
	if commit == "" {
		commit = "HEAD"
	}
	out, err := exec.Command("git", "-C", repo, "rev-parse", commit).Output()
	if err != nil {
		return "", fmt.Errorf("resolve commit %s: %w", commit, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// newWorktree checks out commit into a fresh detached worktree.
func newWorktree(repo, commit string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "console-bench-*")
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Join(parent, "repo")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", dir, commit).CombinedOutput(); err != nil {
		os.RemoveAll(parent)
		return "", nil, fmt.Errorf("worktree add: %v: %s", err, out)
	}
	cleanup := func() {
		exec.Command("git", "-C", repo, "worktree", "remove", "--force", dir).Run()
		os.RemoveAll(parent)
	}
	return dir, cleanup, nil
}

func shell(ctx context.Context, dir, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func tail(s string) string {
	const max = 2000
	if len(s) > max {
		return s[len(s)-max:]
	}
	return s
}
