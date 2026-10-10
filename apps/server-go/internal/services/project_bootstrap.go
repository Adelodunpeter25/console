// Project bootstrap: the two ways a project comes into existence from the
// New-project dialog — a blank directory with `git init`, or a clone of a
// remote repo. See docs/plan/new-project-dialog-plan.md.
//
// Self-contained on purpose, and separate from GitService (git_service.go)
// and ProjectService (project_service.go): both of those are pure readers.
// This one creates directories on disk, so it is the only place in the
// package that can leave a half-made project behind if it fails partway —
// which is exactly why the target-dir guard runs before any git call.
//
// Registering the resulting project row is deliberately NOT done here. The
// caller composes this with ProjectService.Create, so a failed bootstrap
// never leaves an orphaned DB row pointing at a missing directory.
package services

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

var (
	// ErrTargetNotEmpty is returned when the destination directory already
	// exists and has content in it. The dialog surfaces this verbatim, so it
	// is deliberately phrased as an instruction rather than a diagnosis.
	ErrTargetNotEmpty = errors.New("target folder already exists and is not empty; choose another name or folder")

	// ErrGitHubNotConnected is returned when a clone needs a stored token and
	// none exists. It is a distinct sentinel (not a wrapped git failure) so
	// the route can answer 409 and the dialog can point at Accounts instead
	// of showing an opaque git error.
	ErrGitHubNotConnected = errors.New("GitHub is not connected. Add a personal access token in Settings > Accounts, then try again")

	// ErrInvalidRepoURL is returned for a URL we refuse to hand to git.
	ErrInvalidRepoURL = errors.New("enter a valid GitHub repository URL")
)

type ProjectBootstrapService struct{}

func NewProjectBootstrapService() *ProjectBootstrapService { return &ProjectBootstrapService{} }

// InitBlank creates <parentDir>/<name> and runs `git init` in it. No initial
// commit: "blank project" means an empty repo the agent commits into, not a
// repo with a seed commit it did not ask for.
func (s *ProjectBootstrapService) InitBlank(parentDir, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("enter a project name")
	}
	// Reject a name that would escape parentDir or that has no filesystem
	// representation. filepath.Base is not enough on its own — a name like
	// ".." or containing a separator is a traversal, not a folder.
	if err := validateChildName(name); err != nil {
		return "", err
	}
	parent, err := resolveParentDir(parentDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(parent, name)
	if err := ensureTargetAvailable(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if _, err := runGit(dir, "init", "-b", "main"); err != nil {
		// Never leave a plain directory behind claiming to be a project.
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("could not initialize git repository: %w", err)
	}
	return dir, nil
}

// Clone clones url into <parentDir>/<name>, defaulting <name> to the last
// path segment of the URL. Returns the created directory.
//
// SSH-style GitHub URLs need no handling: gitconfig.Configure() injects
// url.https://github.com/.insteadOf rewrites into os.Environ(), which the
// runGit child inherits, so git@github.com:org/repo already arrives as
// HTTPS and picks up the credential helper on the way.
func (s *ProjectBootstrapService) Clone(url, parentDir, name string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", ErrInvalidRepoURL
	}
	repoName, err := repoNameFromURL(url)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		name = repoName
	}
	if err := validateChildName(name); err != nil {
		return "", err
	}
	parent, err := resolveParentDir(parentDir)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(parent, name)
	if err := ensureTargetAvailable(dir); err != nil {
		return "", err
	}

	// Check the credential before starting a clone that may take minutes,
	// but only when we cannot rule out needing it. See needsGitHubToken.
	if needsGitHubToken(url) && !github.CredentialExists() {
		return "", ErrGitHubNotConnected
	}

	if _, err := runGit(parent, "clone", "--progress", url, dir); err != nil {
		// git cleans up after a failed clone itself in most cases, but a
		// partial directory is possible (e.g. the network dropped mid-fetch).
		// Removing it keeps the guard above honest on retry.
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("clone failed: %w", err)
	}
	return dir, nil
}

// needsGitHubToken reports whether a clone of this URL might need a stored
// token. A github.com URL always might — the same URL can be public or
// private and we cannot know without asking GitHub, which would turn every
// public clone into a network round trip before any work is done. Non-GitHub
// hosts are left entirely alone: this feature's credentials are
// GitHub-specific, so refusing a gitlab.com clone for want of a GitHub token
// would be wrong.
func needsGitHubToken(url string) bool {
	host := hostFromURL(url)
	return host == "github.com"
}

// hostFromURL extracts a lowercase hostname from an HTTPS or SSH-style Git
// URL. Returns "" when the shape is not recognized.
func hostFromURL(url string) string {
	if host, _, found := strings.Cut(strings.TrimPrefix(url, "https://"), "/"); found {
		return normalizeHost(host)
	}
	if host, _, found := strings.Cut(strings.TrimPrefix(url, "http://"), "/"); found {
		return normalizeHost(host)
	}
	if host, _, found := strings.Cut(strings.TrimPrefix(url, "ssh://git@"), "/"); found {
		return normalizeHost(host)
	}
	if rest, found := strings.CutPrefix(url, "git@"); found {
		if host, _, ok := strings.Cut(rest, ":"); ok {
			return normalizeHost(host)
		}
	}
	return ""
}

func normalizeHost(host string) string {
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.ToLower(strings.TrimSpace(host))
}

// repoNameFromURL derives a folder name from the URL's last path segment.
// A trailing ".git" is dropped, since nobody wants a folder called "repo.git".
func repoNameFromURL(url string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(url), "/")
	if h, _, found := strings.Cut(trimmed, "://"); found {
		trimmed = h
	}
	if rest, found := strings.CutPrefix(trimmed, "git@"); found {
		// scp-style: everything after the colon is the path.
		if _, pathPart, ok := strings.Cut(rest, ":"); ok {
			trimmed = pathPart
		}
	} else if _, after, found := strings.Cut(trimmed, "github.com/"); found {
		trimmed = after
	}
	segment := path.Base(trimmed)
	segment = strings.TrimSuffix(segment, ".git")
	if segment == "" || segment == "." || segment == "/" {
		return "", ErrInvalidRepoURL
	}
	return segment, nil
}

// validateChildName rejects names that cannot be a plain child of a parent
// directory. filepath.Join already cleans "..", which would silently move the
// target outside parentDir — so the check is on the raw name, before joining.
func validateChildName(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("enter a project name")
	}
	if strings.ContainsAny(name, `/\`) {
		return errors.New("project name cannot contain slashes; choose a folder instead")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("enter a project name")
	}
	return nil
}

// resolveParentDir validates that the parent directory exists and returns its
// absolute form. Creating parents here would let a typo'd path silently
// scatter projects across the filesystem.
func resolveParentDir(parentDir string) (string, error) {
	trimmed := strings.TrimSpace(parentDir)
	if trimmed == "" {
		return "", errors.New("choose a folder for the project")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("folder does not exist: %s", abs)
	}
	return abs, nil
}

// ensureTargetAvailable allows a missing dir or an existing empty one, and
// refuses anything else. An existing empty dir is legal because macOS users
// routinely pre-create folders before pointing a clone at them.
func ensureTargetAvailable(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrTargetNotEmpty
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrTargetNotEmpty, dir)
	}
	return nil
}