// Worktree operations via the git CLI, one worktree per session.
// See docs/plan/worktrees-plan.md. Self-contained: no integration with
// SessionService or routes yet — session create/delete wiring comes later.
//
// Worktree code lives here, not in GitService (git_service.go), on purpose:
// git stays status/diff/branches/checkout, worktrees stay in this service.
package services

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrWorktreeDirty is returned when removing a worktree that has
	// uncommitted changes (tracked or untracked) without force.
	ErrWorktreeDirty = errors.New("worktree has uncommitted changes")
	// ErrUnbornHEAD is returned when creating a worktree in a repo with
	// no commits yet — git has no base to branch from.
	ErrUnbornHEAD = errors.New("repository has no commits yet; commit first")
)

type WorktreeService struct{}

func NewWorktreeService() *WorktreeService { return &WorktreeService{} }

// WorktreeInfo is one entry from `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path   string
	HEAD   string
	Branch string // empty when detached
	Bare   bool
}

// DefaultRoot returns the central worktree root: $HOME/console/worktrees.
// Decided in docs/plan/worktrees-plan.md §8 (not ~/.console, not in-repo).
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "console", "worktrees"), nil
}

// WorktreeAdd creates <branch> and checks it out at path.
// Refuses repos with no commits yet (unborn HEAD).
func (s *WorktreeService) WorktreeAdd(repoDir, path, branch string) error {
	if _, err := runGit(repoDir, "rev-parse", "--verify", "HEAD"); err != nil {
		return ErrUnbornHEAD
	}
	_, err := runGit(repoDir, "worktree", "add", "-b", branch, path)
	return err
}

// WorktreeList parses `git worktree list --porcelain` for repoDir.
func (s *WorktreeService) WorktreeList(repoDir string) ([]WorktreeInfo, error) {
	out, err := runGit(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	infos := make([]WorktreeInfo, 0)
	var cur *WorktreeInfo
	flush := func() {
		if cur != nil && cur.Path != "" {
			infos = append(infos, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &WorktreeInfo{Path: strings.TrimSpace(strings.TrimPrefix(line, "worktree "))}
		case cur != nil && strings.HasPrefix(line, "HEAD "):
			cur.HEAD = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case cur != nil && strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			cur.Branch = strings.TrimPrefix(cur.Branch, "refs/heads/")
		case cur != nil && strings.TrimSpace(line) == "bare":
			cur.Bare = true
		case strings.TrimSpace(line) == "":
			flush()
		}
	}
	flush()
	return infos, nil
}

// IsDirty reports whether the worktree at path has uncommitted changes.
// Untracked files count — an agent's half-written work is still work.
func (s *WorktreeService) IsDirty(path string) (bool, error) {
	out, err := runGit(path, "status", "--porcelain=v1", "-u")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// WorktreeRemove removes the worktree at path. Refuses dirty worktrees
// unless force is true — never silently destroys work.
func (s *WorktreeService) WorktreeRemove(repoDir, path string, force bool) error {
	if !force {
		dirty, err := s.IsDirty(path)
		if err != nil {
			return err
		}
		if dirty {
			return ErrWorktreeDirty
		}
	}
	args := []string{"worktree", "remove", path}
	if force {
		args = append(args, "--force")
	}
	_, err := runGit(repoDir, args...)
	return err
}

// WorktreePrune clears stale worktree metadata (e.g. after a crash left
// a worktree dir behind without its admin files).
func (s *WorktreeService) WorktreePrune(repoDir string) error {
	_, err := runGit(repoDir, "worktree", "prune")
	return err
}
