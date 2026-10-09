// Files-to-copy for new worktrees. See docs/plan/worktree-files-to-copy-plan.md.
//
// New git worktrees start with tracked files only, so gitignored local
// files (signing keys, .env*, local config) never arrive on their own.
// After WorktreeAdd succeeds, the patterns from the source checkout's
// .worktreeinclude (or the default .env*) decide which gitignored files get
// carried over: plain entries are copied (snapshot), `link:` entries are
// symlinked (live).
//
// Everything here is best-effort: any failure carries nothing extra and
// worktree creation proceeds exactly as if there were nothing to carry.
// Never log file contents — entries like key.properties hold passwords,
// so logs carry counts only.
package services

import (
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// worktreeIncludeFile is the committed, tool-agnostic pattern file read
// from the source checkout's root (Claude Code reads the same file).
const worktreeIncludeFile = ".worktreeinclude"

// skippedWorktreeSegs are never carried over even when matched: large,
// regenerable dependency and build output that would slow creation down
// and drag stale state into a fresh worktree.
var skippedWorktreeSegs = []string{
	"node_modules", "build", "dist", "target", ".gradle", ".next",
}

// includePattern is one parsed .worktreeinclude line: gitignore syntax
// plus the Console `link:` extension (symlink instead of copy).
type includePattern struct {
	link     bool
	negate   bool
	dirOnly  bool
	anchored bool // contains a slash: rooted, per gitignore rules
	literal  string
	segs     []string
}

// parseWorktreeInclude parses .worktreeinclude contents. Blank lines and
// `#` comments are ignored; `!` negates; a trailing `/` matches dirs
// only; `link:` marks the entry for symlinking. Garbage lines parse to
// patterns that match nothing — they must never break creation.
func parseWorktreeInclude(data string) []includePattern {
	var out []includePattern
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		var p includePattern
		if strings.HasPrefix(line, "link:") {
			p.link = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "link:"))
			if line == "" {
				continue
			}
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\!`) || strings.HasPrefix(line, `\#`) {
			line = line[1:]
		} else if strings.HasPrefix(line, "!") {
			p.negate = true
			line = line[1:]
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		line = strings.TrimPrefix(line, "/")
		if line == "" {
			continue
		}
		p.literal = line
		p.anchored = strings.Contains(line, "/")
		p.segs = strings.Split(line, "/")
		out = append(out, p)
	}
	return out
}

// hasMagic reports whether the pattern needs glob matching (as opposed to
// a root-relative literal path, used for whole-dir symlinks).
func (p includePattern) hasMagic() bool {
	return strings.ContainsAny(p.literal, "*?[")
}

// matchSegments matches pattern segments against path segments, where a
// `**` segment crosses any number of segments (including zero).
func matchSegments(pat, target []string) bool {
	if len(pat) == 0 {
		return len(target) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(target); i++ {
			if matchSegments(pat[1:], target[i:]) {
				return true
			}
		}
		return false
	}
	if len(target) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], target[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pat[1:], target[1:])
}

// matches reports whether the gitignore-style pattern matches rel (a
// slash-separated, root-relative file path). A pattern naming a directory
// also matches everything under it. Bad glob syntax matches nothing.
func (p includePattern) matches(rel string) bool {
	segs := strings.Split(rel, "/")
	if !p.anchored {
		for i, seg := range segs {
			ok, err := path.Match(p.segs[0], seg)
			if err != nil || !ok {
				continue
			}
			if !p.dirOnly || i < len(segs)-1 {
				return true
			}
		}
		return false
	}
	for k := 1; k <= len(segs); k++ {
		if matchSegments(p.segs, segs[:k]) {
			if !p.dirOnly || k < len(segs) {
				return true
			}
		}
	}
	return false
}

// resolveWorktreePatterns returns the patterns for the checkout rooted at
// top: its .worktreeinclude when present, otherwise the default .env*.
func resolveWorktreePatterns(top string) []includePattern {
	if data, err := os.ReadFile(filepath.Join(top, worktreeIncludeFile)); err == nil {
		return parseWorktreeInclude(string(data))
	}
	return parseWorktreeInclude(".env*\n")
}

// gitIsIgnored reports whether p (root-relative) is ignored in the
// checkout rooted at top. Any git failure counts as "not ignored":
// eligibility must stay conservative and best-effort.
func gitIsIgnored(top, p string) bool {
	_, err := runGit(top, "check-ignore", "-q", p)
	return err == nil
}

// hasSkippedSeg reports whether rel passes through dependency or build
// output that is never carried over, even when matched.
func hasSkippedSeg(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		for _, skip := range skippedWorktreeSegs {
			if seg == skip {
				return true
			}
		}
	}
	return false
}

// copyWorktreeFile snapshots the source-checkout file rel into the new
// worktree, preserving mode bits. Never overwrites an existing dest.
func copyWorktreeFile(top, wtPath, rel string) error {
	src := filepath.Join(top, filepath.FromSlash(rel))
	dst := filepath.Join(wtPath, filepath.FromSlash(rel))
	if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	perm := info.Mode().Perm()
	if perm == 0 {
		perm = 0o644
	}
	return os.WriteFile(dst, data, perm)
}

// linkWorktreePath symlinks the source-checkout path rel (file or whole
// directory) into the new worktree, so later changes at the source (a
// rotated key) are picked up everywhere. Never overwrites an existing
// dest; a missing source is skipped.
func linkWorktreePath(top, wtPath, rel string) error {
	src := filepath.Join(top, filepath.FromSlash(rel))
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	dst := filepath.Join(wtPath, filepath.FromSlash(rel))
	if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Symlink(abs, dst)
}

// carryWorktreeFiles carries the source checkout's gitignored local files
// matching its patterns into the new worktree at wtPath. It returns copy
// and link counts; any error aborts the carry-over (the caller swallows
// it — creation must proceed exactly as before).
func carryWorktreeFiles(repoDir, wtPath string) (copied, linked int, err error) {
	out, err := runGit(repoDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return 0, 0, err
	}
	top := strings.TrimSpace(out)
	patterns := resolveWorktreePatterns(top)

	// Root-relative literal `link:` entries first: a literal naming a
	// directory symlinks the whole directory, so later additions at the
	// source (a rotated keystore) show up in every worktree.
	for _, p := range patterns {
		if !p.link || p.negate || p.hasMagic() {
			continue
		}
		if hasSkippedSeg(p.literal) || !gitIsIgnored(top, p.literal) {
			continue
		}
		if err := linkWorktreePath(top, wtPath, p.literal); err != nil {
			continue
		}
		linked++
	}

	// Glob entries match against the checkout's ignored files.
	out, err = runGit(top, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return copied, linked, err
	}
	for _, rel := range strings.Split(out, "\x00") {
		rel = strings.TrimSpace(rel)
		if rel == "" || hasSkippedSeg(rel) {
			continue
		}
		op := 0 // 1 = copy, 2 = link
		for _, p := range patterns {
			if !p.matches(rel) {
				continue
			}
			switch {
			case p.negate:
				op = 0
			case p.link:
				op = 2
			default:
				op = 1
			}
		}
		switch op {
		case 1:
			if err := copyWorktreeFile(top, wtPath, rel); err == nil {
				copied++
			}
		case 2:
			if err := linkWorktreePath(top, wtPath, rel); err == nil {
				linked++
			}
		}
	}
	return copied, linked, nil
}

// carryWorktreeFilesBestEffort runs the files-to-copy carry-over and
// swallows every failure: a failed carry-over must be indistinguishable
// from not having one. Counts only — never file contents — hit the logs.
func (s *WorktreeService) carryWorktreeFilesBestEffort(repoDir, wtPath string) {
	copied, linked, err := carryWorktreeFiles(repoDir, wtPath)
	if err != nil {
		slog.Debug("worktree files-to-copy skipped", "repo", repoDir, "error", err)
		return
	}
	if copied+linked > 0 {
		slog.Debug("worktree files-to-copy done", "repo", repoDir, "copied", copied, "linked", linked)
	}
}
