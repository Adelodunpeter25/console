// Repo resolution for pull requests: a project's PR repo is its origin
// remote, parsed to owner/repo (docs/plan/pull-requests.md §8, decided).
// origin only, no base_repo argument — the author does not work from forks.
package services

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ResolveRepo maps a registered project to its GitHub owner/repo via the
// origin remote. A worktree shares its repo's remote, so it resolves the
// same way. Non-github.com remotes are a clear error, not a silent
// fallthrough.
func (s *PullRequestService) ResolveRepo(projectID string) (string, string, error) {
	if strings.TrimSpace(projectID) == "" {
		return "", "", errors.New("project_id is required for scope=repo.")
	}
	project, err := s.projects.Get(projectID)
	if err != nil {
		return "", "", fmt.Errorf("Unknown project %q.", projectID)
	}
	out, err := exec.Command("git", "-C", project.Path, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", errors.New("This project has no origin remote, so it has no pull requests.")
	}
	owner, repo, ok := ParseGitHubRemote(strings.TrimSpace(string(out)))
	if !ok {
		return "", "", errors.New("This project's origin is not a github.com repository.")
	}
	return owner, repo, nil
}

// ParseGitHubRemote extracts owner/repo from a github.com remote URL —
// HTTPS, SSH, and ssh:// forms all resolve; other hosts do not.
func ParseGitHubRemote(remote string) (string, string, bool) {
	r := strings.TrimSuffix(remote, ".git")
	r = strings.TrimSuffix(r, "/")
	// https://github.com/owner/repo, ssh://git@github.com/owner/repo
	if i := strings.Index(r, "github.com/"); i >= 0 {
		r = r[i+len("github.com/"):]
	} else if i := strings.Index(r, "github.com:"); i >= 0 {
		// git@github.com:owner/repo
		r = r[i+len("github.com:"):]
	} else {
		return "", "", false
	}
	parts := strings.Split(r, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
