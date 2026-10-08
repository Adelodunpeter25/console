// Git operations via the git CLI (status, diff, branches,
// checkout).
package services

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type GitService struct {
	mu       sync.Mutex
	inflight map[string]*statusCall
}

// statusCall lets concurrent GetGitStatus calls for one repo share a single
// computation: several watchers on the same checkout used to each spawn their
// own git processes for identical results.
type statusCall struct {
	done   chan struct{}
	result types.GitStatusSummary
}

func NewGitService() *GitService { return &GitService{inflight: make(map[string]*statusCall)} }

// runGit runs a git command in dir and returns its stdout. On failure the
// error carries git's stderr — without it a caller only sees "exit status
// 128/255", which tells neither the user nor the retry logic what went wrong.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return string(out), fmt.Errorf("%w: %s", err, msg)
		}
	}
	return string(out), err
}

// GetGitStatus returns branch + porcelain file entries with numstat deltas.
// It runs two git processes (status and one combined numstat) instead of four,
// with --no-optional-locks so polling never contends with the user's own git
// commands for the index lock.
func (s *GitService) GetGitStatus(repoPath string) types.GitStatusSummary {
	s.mu.Lock()
	if call, ok := s.inflight[repoPath]; ok {
		s.mu.Unlock()
		<-call.done
		return call.result
	}
	call := &statusCall{done: make(chan struct{})}
	s.inflight[repoPath] = call
	s.mu.Unlock()

	call.result = s.computeGitStatus(repoPath)
	s.mu.Lock()
	delete(s.inflight, repoPath)
	s.mu.Unlock()
	close(call.done)
	return call.result
}

func (s *GitService) computeGitStatus(repoPath string) types.GitStatusSummary {
	statusOut, err := runGit(repoPath, "--no-optional-locks", "status", "--porcelain=v1", "--branch", "-u")
	if err != nil {
		// Not a git repository or git failed — clean empty summary.
		return types.GitStatusSummary{Branch: "", Clean: true, Files: []types.GitFileEntry{}}
	}
	branch, statusOut := splitStatusBranch(statusOut)
	numstat := s.numstatMap(repoPath)

	if branch == "" {
		branch = "main"
	}
	files := make([]types.GitFileEntry, 0)
	for _, line := range strings.Split(statusOut, "\n") {
		if strings.TrimSpace(line) == "" || len(line) < 4 {
			continue
		}
		indexStatus, workTreeStatus := line[0], line[1]
		rawFilePath := strings.TrimSpace(line[3:])
		filePath := rawFilePath
		if idx := strings.Index(rawFilePath, "->"); idx >= 0 {
			filePath = strings.TrimSpace(rawFilePath[idx+2:])
		}
		absPath, _ := filepath.Abs(filepath.Join(repoPath, filePath))

		var status types.GitFileStatus = "?"
		staged := false
		switch {
		case indexStatus == '?' && workTreeStatus == '?':
			status = "?"
		case indexStatus == 'A' || workTreeStatus == 'A':
			status, staged = "A", indexStatus == 'A'
		case indexStatus == 'M' || workTreeStatus == 'M':
			status, staged = "M", indexStatus == 'M'
		case indexStatus == 'D' || workTreeStatus == 'D':
			status, staged = "D", indexStatus == 'D'
		case indexStatus == 'R' || workTreeStatus == 'R':
			status = "R"
		}
		stats, ok := numstat[filePath]
		if !ok {
			stats, ok = numstat[rawFilePath]
		}
		additions, deletions := int64(0), int64(0)
		if ok {
			additions, deletions = stats[0], stats[1]
		}
		files = append(files, types.GitFileEntry{
			Path: absPath, Status: status, Staged: staged,
			Additions: additions, Deletions: deletions,
		})
	}
	return types.GitStatusSummary{Branch: branch, Clean: len(files) == 0, Files: files}
}

// numstatMap returns per-file added/deleted line counts for everything that
// differs from HEAD (staged and unstaged together). A repo with no commits has
// no HEAD, so the map is simply empty there.
func (s *GitService) numstatMap(repoPath string) map[string][2]int64 {
	m := make(map[string][2]int64)
	if out, err := runGit(repoPath, "--no-optional-locks", "diff", "HEAD", "--numstat"); err == nil {
		parseNumstat(out, m)
	}
	return m
}

// splitStatusBranch pulls the "## branch...upstream [ahead 1]" header line
// off `status --branch` output and returns the branch name plus the file lines.
func splitStatusBranch(out string) (string, string) {
	header, rest, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(header, "## ") {
		return "", out
	}
	name := strings.TrimPrefix(header, "## ")
	switch {
	case strings.HasPrefix(name, "No commits yet on "):
		name = strings.TrimPrefix(name, "No commits yet on ")
	case strings.HasPrefix(name, "HEAD (no branch)"):
		name = "HEAD"
	}
	if i := strings.Index(name, "..."); i >= 0 {
		name = name[:i]
	}
	if i := strings.IndexByte(name, ' '); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name), rest
}

func parseNumstat(out string, m map[string][2]int64) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		var add, del int64
		// Binary files show "-" for both counts.
		if fields[0] != "-" {
			add, _ = strconv.ParseInt(fields[0], 10, 64)
		}
		if fields[1] != "-" {
			del, _ = strconv.ParseInt(fields[1], 10, 64)
		}
		m[fields[2]] = [2]int64{add, del}
	}
}

// GetDiff returns the unified diff for the repo or a single file.
func (s *GitService) GetDiff(repoPath string, filePath string) (string, error) {
	cwd := repoPath
	target := ""
	if filePath != "" {
		abs, _ := filepath.Abs(filePath)
		if repoPath == "" {
			if root, err := runGit(filepath.Dir(abs), "rev-parse", "--show-toplevel"); err == nil {
				cwd = strings.TrimSpace(root)
			} else {
				cwd = filepath.Dir(abs)
			}
		}
		target = " -- \"" + abs + "\""
	}
	if out, err := runGit(cwd, "diff", "HEAD"+strings.TrimPrefix(target, " --")); err == nil && out != "" {
		return out, nil
	}
	out, err := runGit(cwd, "diff")
	if err != nil {
		return "", err
	}
	if out == "" && filePath != "" {
		// Untracked file: synthesize a whole-file diff.
		if status, err := runGit(cwd, "status", "--porcelain", "--", filePath); err == nil && strings.TrimSpace(status) != "" {
			if noIndex, err := runGit(cwd, "diff", "--no-index", "/dev/null", filePath); err == nil {
				return noIndex, nil
			}
		}
	}
	return out, nil
}

// ListBranches lists local branches with the checked-out one marked.
func (s *GitService) ListBranches(repoPath string) types.GitBranchesResponse {
	if isRepo, err := runGit(repoPath, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(isRepo) != "true" {
		return types.GitBranchesResponse{Branches: []types.GitBranchInfo{}, IsGitRepository: false}
	}
	out, _ := runGit(repoPath, "branch", "--format=%(refname:short)")
	currentOut, _ := runGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	current := strings.TrimSpace(currentOut)
	branches := make([]types.GitBranchInfo, 0)
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		branches = append(branches, types.GitBranchInfo{Name: name, Current: name == current})
	}
	return types.GitBranchesResponse{Branches: branches, IsGitRepository: true}
}

func (s *GitService) CheckoutBranch(repoPath, branch string) error {
	_, err := runGit(repoPath, "switch", branch)
	return err
}
