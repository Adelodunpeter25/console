// Lightweight workspace tree for the system prompt. Port of
// apps/server/agent/src/systemprompt/workspace-tree.ts.
package systemprompt

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	treeDefaultMaxDepth    = 3
	treeDefaultPerDirLimit = 12
	treeDefaultLineCap     = 120
)

var treeSkipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".hg": true, ".svn": true,
	"dist": true, "build": true, "out": true, "coverage": true,
	".next": true, ".turbo": true, ".cache": true, "__pycache__": true,
	".venv": true, "venv": true, "target": true,
}

// WorkspaceTree is the rendered directory listing shown in the prompt.
type WorkspaceTree struct {
	RootPath   string
	Rendered   string
	Truncated  bool
	TotalLines int
}

type treeEntry struct {
	name     string
	isDir    bool
	children []treeEntry
}

func readTreeChildren(dir string, depth, maxDepth, perDirLimit int, showHidden bool) ([]treeEntry, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	filtered := make([]os.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() && treeSkipDirs[e.Name()] {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].IsDir() != filtered[j].IsDir() {
			return filtered[i].IsDir()
		}
		return filtered[i].Name() < filtered[j].Name()
	})
	truncated := false
	if len(filtered) > perDirLimit {
		filtered = filtered[:perDirLimit]
		truncated = true
	}
	out := make([]treeEntry, 0, len(filtered))
	for _, e := range filtered {
		if e.IsDir() {
			child := treeEntry{name: e.Name(), isDir: true}
			if depth < maxDepth {
				nested, nestedTruncated := readTreeChildren(filepath.Join(dir, e.Name()), depth+1, maxDepth, perDirLimit, showHidden)
				child.children = nested
				truncated = truncated || nestedTruncated
			}
			out = append(out, child)
		} else {
			out = append(out, treeEntry{name: e.Name()})
		}
	}
	return out, truncated
}

func renderTree(entries []treeEntry, prefix string, lines *[]string, lineCap int) bool {
	for i, entry := range entries {
		if len(*lines) >= lineCap {
			return true
		}
		isLast := i == len(entries)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}
		suffix := ""
		if entry.isDir {
			suffix = "/"
		}
		*lines = append(*lines, prefix+connector+entry.name+suffix)
		if len(entry.children) > 0 {
			if renderTree(entry.children, childPrefix, lines, lineCap) {
				return true
			}
		}
	}
	return false
}

// BuildWorkspaceTree renders a depth-limited directory tree rooted at cwd.
func BuildWorkspaceTree(cwd string) WorkspaceTree {
	root, err := filepath.Abs(cwd)
	if err != nil {
		root = cwd
	}
	entries, dirTruncated := readTreeChildren(root, 1, treeDefaultMaxDepth, treeDefaultPerDirLimit, false)
	lines := []string{filepath.Base(root) + "/"}
	lineTruncated := renderTree(entries, "", &lines, treeDefaultLineCap)
	truncated := dirTruncated || lineTruncated
	if truncated {
		lines = append(lines, "… (tree truncated — use listDir/glob to drill in)")
	}
	return WorkspaceTree{RootPath: root, Rendered: strings.Join(lines, "\n"), Truncated: truncated, TotalLines: len(lines)}
}
