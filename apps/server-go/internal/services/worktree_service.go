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
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

var (
	// ErrWorktreeDirty is returned when removing a worktree that has
	// uncommitted changes (tracked or untracked) without force.
	ErrWorktreeDirty = errors.New("worktree has uncommitted changes")
	// ErrUnbornHEAD is returned when creating a worktree in a repo with
	// no commits yet — git has no base to branch from.
	ErrUnbornHEAD = errors.New("repository has no commits yet; commit first")
	// ErrNotGitRepo is returned when the target dir is not a git repo.
	ErrNotGitRepo = errors.New("not a git repository")
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

// WorktreeEntry is the API-facing worktree description (owned or orphan).
type WorktreeEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
	Owned  bool   `json:"owned"`
}

// DefaultRoot returns the central worktree root. Centralized in the app
// paths file (utils.WorktreesDir) so dev and prod differ — see the
// resolved-decisions note in docs/plan/worktrees-plan.md §8.
func DefaultRoot() (string, error) {
	return utils.WorktreesDir(), nil
}

// isGitRepo reports whether dir is inside a git work tree.
func isGitRepo(dir string) bool {
	out, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// WorktreeAdd creates <branch> and checks it out at path, always branching
// from origin/HEAD (the repo's default branch on the remote) so the new
// worktree starts clean regardless of what the local HEAD is.
// Refuses non-repos loudly and repos with no commits yet (unborn HEAD).
func (s *WorktreeService) WorktreeAdd(repoDir, path, branch string) error {
	if !isGitRepo(repoDir) {
		return ErrNotGitRepo
	}
	if _, err := runGit(repoDir, "rev-parse", "--verify", "HEAD"); err != nil {
		return ErrUnbornHEAD
	}
	base := resolveDefaultBranch(repoDir)
	_, err := runGit(repoDir, "worktree", "add", "-b", branch, path, base)
	return err
}

// resolveDefaultBranch returns the remote ref to branch new worktrees from.
// It tries, in order:
//  1. origin/HEAD (set when the repo was cloned — most reliable)
//  2. origin/main
//  3. origin/master
//  4. HEAD (local fallback — same as the old behaviour)
func resolveDefaultBranch(repoDir string) string {
	// Try origin/HEAD first — git sets this on clone and it always points
	// at the remote's default branch.
	if out, err := runGit(repoDir, "rev-parse", "--verify", "origin/HEAD"); err == nil && strings.TrimSpace(out) != "" {
		return "origin/HEAD"
	}
	for _, candidate := range []string{"origin/main", "origin/master"} {
		if out, err := runGit(repoDir, "rev-parse", "--verify", candidate); err == nil && strings.TrimSpace(out) != "" {
			return candidate
		}
	}
	return "HEAD"
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
// unless force is true — never silently destroys work. A path that no
// longer exists on disk is already gone: prune stale metadata and succeed
// so permanent delete can never get stuck behind a missing dir.
//
// Branch cleanup: the branch created for this worktree (pass it explicitly
// when known, or "" to auto-detect via BranchOf) is deleted best-effort
// after the worktree is gone. Safe delete (`-d`) by default so a branch
// with unique commits is kept; force=true uses `-D`. Branch cleanup never
// fails the removal itself.
func (s *WorktreeService) WorktreeRemove(repoDir, path, branch string, force bool) error {
	if branch == "" {
		if b, err := s.BranchOf(path); err == nil {
			branch = b
		}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, _ = runGit(repoDir, "worktree", "prune")
		_ = s.DeleteBranch(repoDir, branch, force)
		return nil
	}
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
	if _, err := runGit(repoDir, args...); err != nil {
		return err
	}
	_ = s.DeleteBranch(repoDir, branch, force)
	_, _ = runGit(repoDir, "worktree", "prune")
	return nil
}

// DeleteBranch removes a worktree-owned branch best-effort. Safe delete
// (`-d`) by default keeps branches with unique unmerged commits; force
// uses `-D`. Empty, detached (""), HEAD, and protected default branches
// (main/master) are never touched. Never returns an error for a missing
// or unmerged branch — only for unexpected git failures under force.
func (s *WorktreeService) DeleteBranch(repoDir, branch string, force bool) error {
	if branch == "" || branch == "HEAD" {
		return nil
	}
	if branch == "main" || branch == "master" {
		return nil
	}
	args := []string{"branch", "-d", branch}
	if force {
		args = []string{"branch", "-D", branch}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.ToLower(string(out) + " " + err.Error())
	if strings.Contains(msg, "not found") ||
		strings.Contains(msg, "not fully merged") ||
		strings.Contains(msg, "is not merged") ||
		strings.Contains(msg, "no branch") ||
		strings.Contains(msg, "unknown revision") {
		return nil
	}
	if !force {
		return nil
	}
	return err
}

// WorktreePrune clears stale worktree metadata (e.g. after a crash left
// a worktree dir behind without its admin files).
func (s *WorktreeService) WorktreePrune(repoDir string) error {
	_, err := runGit(repoDir, "worktree", "prune")
	return err
}

// BranchOf returns the checked-out branch of the worktree at path,
// or "" when detached.
func (s *WorktreeService) BranchOf(path string) (string, error) {
	out, err := runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(out)
	if branch == "HEAD" {
		return "", nil
	}
	return branch, nil
}

// MainRepoDir resolves the main worktree of the repo backing path
// (first entry of `git worktree list`).
func (s *WorktreeService) MainRepoDir(path string) (string, error) {
	infos, err := s.WorktreeList(path)
	if err != nil {
		return "", err
	}
	if len(infos) == 0 {
		return "", errors.New("no worktrees found")
	}
	return infos[0].Path, nil
}

// ScanOrphans lists dirs directly under root that no session owns.
// Unreadable dirs are reported with empty branch, clean=false certainty off
// (Dirty=false) — deletion without force still refuses when state cannot
// be verified (see RemoveOrphan).
func (s *WorktreeService) ScanOrphans(root string, owned []string) ([]WorktreeEntry, error) {
	ownedSet := make(map[string]bool, len(owned))
	for _, p := range owned {
		if abs, err := filepath.Abs(p); err == nil {
			ownedSet[abs] = true
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []WorktreeEntry{}, nil
		}
		return nil, err
	}
	out := make([]WorktreeEntry, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(root, e.Name()))
		if err != nil {
			continue
		}
		if ownedSet[abs] {
			continue
		}
		entry := WorktreeEntry{Path: abs}
		if branch, err := s.BranchOf(abs); err == nil {
			entry.Branch = branch
		}
		if dirty, err := s.IsDirty(abs); err == nil {
			entry.Dirty = dirty
		}
		out = append(out, entry)
	}
	return out, nil
}

// RemoveOrphan removes a worktree dir with no owning session. The path must
// live under root (containment — never touch dirs outside the worktree
// root). Dirty still blocks without force; force on an unreadable worktree
// falls back to deleting the dir. The worktree's branch follows the same
// safe/force rule as WorktreeRemove (safe `-d` keeps unmerged work).
func (s *WorktreeService) RemoveOrphan(root, path string, force bool) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("path is outside the worktree root")
	}
	branch, _ := s.BranchOf(absPath)
	repoDir, err := s.MainRepoDir(absPath)
	if err != nil {
		if !force {
			return err
		}
		_ = os.RemoveAll(absPath)
		return nil
	}
	if err := s.WorktreeRemove(repoDir, absPath, branch, force); err != nil {
		return err
	}
	_, _ = runGit(repoDir, "worktree", "prune")
	return nil
}
