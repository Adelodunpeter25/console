// Filesystem operations. Port of apps/server/api/src/services/fs.service.ts
// and the preview gating rules in packages/types/src/fs.ts.
package services

import (
	"bytes"
	"fmt"

	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	maxFilePreviewBytes = 512 * 1024
	imageMaxBytes       = 10 * 1024 * 1024
)

// manager is set once at startup; nil means fff is not wired.
var manager *fff.Manager

// SetFffManager wires the fff index manager into the fs service.
func SetFffManager(m *fff.Manager) { manager = m }

// PreviewBlocked carries the structured code/status the /file routes emit.
type PreviewBlocked struct {
	Code   string // LOCKFILE_BLOCKED | BINARY_FILE | FILE_TOO_LARGE
	Status int
	Detail map[string]any
	Msg    string
}

func (e *PreviewBlocked) Error() string { return e.Msg }

func BuildFileETag(sizeBytes int64, mtimeMs int64) string {
	return fmt.Sprintf("W/\"%s-%s\"",
		strconv.FormatInt(sizeBytes, 36),
		strconv.FormatInt(mtimeMs, 36))
}

var lockFileBasenames = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true,
	"pnpm-lock.yaml": true, "composer.lock": true,
}

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
	".ico": true, ".icns": true, ".tiff": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".webm": true,
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true, ".jar": true, ".war": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".pdf": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".lock": true, ".lockb": true,
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".ico": true, ".svg": true,
}

type FsService struct{}

func NewFsService() *FsService { return &FsService{} }

// checkPreviewGate returns a *PreviewBlocked when the file cannot be shown.
func checkPreviewGate(path string, size int64) *PreviewBlocked {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	if lockFileBasenames[base] || ext == ".lock" || ext == ".lockb" {
		return &PreviewBlocked{Code: "LOCKFILE_BLOCKED", Status: 403,
			Msg: fmt.Sprintf("Machine-generated lockfile %q cannot be previewed.", base)}
	}
	if binaryExtensions[ext] {
		return &PreviewBlocked{Code: "BINARY_FILE", Status: 403,
			Msg: fmt.Sprintf("Binary file %q cannot be previewed as text.", base)}
	}
	if size > maxFilePreviewBytes {
		return &PreviewBlocked{Code: "FILE_TOO_LARGE", Status: 413,
			Msg:    fmt.Sprintf("%q is %s — previews are capped at %s.", base, formatBytes(size), formatBytes(maxFilePreviewBytes)),
			Detail: map[string]any{"sizeBytes": size, "maxBytes": maxFilePreviewBytes}}
	}
	return nil
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// BrowseDirectory lists one directory for the file picker UI.
func (s *FsService) BrowseDirectory(targetPath string, showHidden bool) (types.FsBrowseResult, error) {
	home, _ := os.UserHomeDir()
	resolved := targetPath
	if resolved == "" {
		resolved = home
	}
	resolved, err := filepath.Abs(resolved)
	if err != nil {
		return types.FsBrowseResult{}, err
	}
	parent := filepath.Dir(resolved)
	var parentPath *string
	if parent != resolved {
		parentPath = &parent
	}

	dirEntries, err := os.ReadDir(resolved)
	if err != nil {
		return types.FsBrowseResult{}, err
	}
	entries := make([]types.FsTreeEntry, 0)
	for _, de := range dirEntries {
		if !showHidden && utils.IsHiddenName(de.Name()) {
			continue
		}
		entry := types.FsTreeEntry{
			Name:  de.Name(),
			Path:  filepath.Join(resolved, de.Name()),
			IsDir: de.IsDir(),
		}
		if !de.IsDir() && !utils.IsPathIgnored(de.Name()) {
			if info, err := de.Info(); err == nil {
				size := info.Size()
				entry.Size = &size
			}
		}
		entries = append(entries, entry)
	}
	sortEntries(entries)
	return types.FsBrowseResult{CurrentPath: resolved, ParentPath: parentPath, Entries: entries}, nil
}

func sortEntries(entries []types.FsTreeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return numericLess(entries[i].Name, entries[j].Name)
	})
}

// numericLess approximates locale-aware numeric alphabetical ordering.
func numericLess(a, b string) bool {
	return strings.Compare(a, b) < 0
}

// EntriesOptions is the exported form used by route handlers.
type EntriesOptions struct {
	WithSizes  bool
	MaxEntries int
}

// ListAllEntries walks a tree DFS-style: directories first, each directory
// immediately followed by its subtree, capped at maxEntries.
func (s *FsService) ListAllEntries(targetPath string, maxDepth int, showHidden bool, opts EntriesOptions) ([]types.FsTreeEntry, error) {
	resolved, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, err
	}
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = 30000
	}
	counter := 0
	truncated := false
	var walk func(dir string, depth int) []types.FsTreeEntry
	walk = func(dir string, depth int) []types.FsTreeEntry {
		if depth > maxDepth || truncated {
			return nil
		}
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		visible := make([]os.DirEntry, 0, len(dirEntries))
		for _, de := range dirEntries {
			if !showHidden && utils.IsHiddenName(de.Name()) {
				continue
			}
			if !showHidden && utils.IsPathIgnored(de.Name()) {
				continue
			}
			visible = append(visible, de)
		}
		sort.Slice(visible, func(i, j int) bool {
			if visible[i].IsDir() != visible[j].IsDir() {
				return visible[i].IsDir()
			}
			return numericLess(visible[i].Name(), visible[j].Name())
		})

		local := make([]types.FsTreeEntry, 0, len(visible))
		for _, de := range visible {
			if truncated || counter >= opts.MaxEntries {
				truncated = true
				break
			}
			counter++
			entry := types.FsTreeEntry{
				Name:  de.Name(),
				Path:  filepath.Join(dir, de.Name()),
				IsDir: de.IsDir(),
			}
			if opts.WithSizes && !de.IsDir() {
				if info, err := de.Info(); err == nil {
					size := info.Size()
					entry.Size = &size
				}
			}
			local = append(local, entry)
		}
		// Interleave children DFS-style after their parent directory.
		for i, entry := range local {
			if !entry.IsDir {
				continue
			}
			children := walk(entry.Path, depth+1)
			local[i].Children = children
			if len(children) > 0 {
				out := make([]types.FsTreeEntry, 0, len(local)+len(children))
				out = append(out, local[:i+1]...)
				out = append(out, children...)
				out = append(out, local[i+1:]...)
				local = out
			}
		}
		return local
	}
	return walk(resolved, 1), nil
}

// GetDirectoryTree renders the list-dir tool's tree format.
func (s *FsService) GetDirectoryTree(targetPath string, maxDepth int, showHidden bool) (string, error) {
	entries, err := s.ListAllEntries(targetPath, maxDepth, showHidden, EntriesOptions{WithSizes: false, MaxEntries: 3000})
	if err != nil {
		return "", err
	}
	depthNote := fmt.Sprintf(" (recursive, max depth %d)", maxDepth)
	header := fmt.Sprintf("Directory: %s%s\n", targetPath, depthNote)
	if len(entries) == 0 {
		return header + "(empty directory)", nil
	}
	var buf bytes.Buffer
	renderTree(&buf, entries, "")
	return header + buf.String(), nil
}

func renderTree(buf *bytes.Buffer, entries []types.FsTreeEntry, prefix string) {
	for i, entry := range entries {
		isLast := i == len(entries)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}
		if entry.IsDir {
			buf.WriteString(prefix + connector + entry.Name + "/\n")
			if len(entry.Children) == 0 {
				buf.WriteString(prefix + childPrefix + "(empty)\n")
			} else {
				renderTree(buf, entry.Children, prefix+childPrefix)
			}
		} else {
			sizeStr := ""
			if entry.Size != nil {
				sizeStr = " (" + formatBytes(*entry.Size) + ")"
			}
			buf.WriteString(prefix + connector + entry.Name + sizeStr + "\n")
		}
	}
}

type FileContentMeta struct {
	Content   string `json:"content"`
	SizeBytes int64  `json:"sizeBytes"`
	MtimeMs   int64  `json:"mtimeMs"`
}

// ReadFileContentWithMeta reads text content with preview gating.
func (s *FsService) ReadFileContentWithMeta(path string, startLine, endLine int) (FileContentMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileContentMeta{}, err
	}
	if blocked := checkPreviewGate(path, info.Size()); blocked != nil {
		return FileContentMeta{}, blocked
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return FileContentMeta{}, err
	}
	content := sliceLines(string(raw), startLine, endLine)
	return FileContentMeta{
		Content:   content,
		SizeBytes: info.Size(),
		MtimeMs:   info.ModTime().UnixMilli(),
	}, nil
}

// sliceLines extracts [startLine, endLine] (1-based, inclusive), matching
// the TS implementation's semantics.
func sliceLines(text string, startLine, endLine int) string {
	if startLine == 0 && endLine == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	count := len(lines)
	start0 := startLine
	if start0 < 1 {
		start0 = 1
	}
	endExcl := endLine
	if endExcl <= 0 || endExcl > count {
		endExcl = count
	}
	if start0-1 >= count || endExcl <= start0-1 {
		return ""
	}
	return strings.Join(lines[start0-1:endExcl], "\n")
}

type ImageMeta struct {
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	MtimeMs   int64  `json:"mtimeMs"`
}

// GetImageMeta validates and describes an image file for raw preview.
func (s *FsService) GetImageMeta(path string) (ImageMeta, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !imageExtensions[ext] {
		return ImageMeta{}, &PreviewBlocked{Code: "BINARY_FILE", Status: 403,
			Msg: fmt.Sprintf("%q is not a previewable image.", filepath.Base(path))}
	}
	info, err := os.Stat(path)
	if err != nil {
		return ImageMeta{}, err
	}
	if info.Size() > imageMaxBytes {
		return ImageMeta{}, &PreviewBlocked{Code: "FILE_TOO_LARGE", Status: 413,
			Msg:    fmt.Sprintf("%q is %s — image previews are capped at %s.", filepath.Base(path), formatBytes(info.Size()), formatBytes(imageMaxBytes)),
			Detail: map[string]any{"sizeBytes": info.Size(), "maxBytes": imageMaxBytes}}
	}
	mimeType := mime.TypeByExtension(ext)
	if ext == ".svg" {
		mimeType = "image/svg+xml"
	}
	return ImageMeta{MimeType: mimeType, SizeBytes: info.Size(), MtimeMs: info.ModTime().UnixMilli()}, nil
}

// WriteFileContent creates or overwrites a file (mkdir -p the parent).
func (s *FsService) WriteFileContent(path, content string) (string, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %s (%d bytes)", path, len(content)), nil
}

func (s *FsService) DeleteFile(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

func (s *FsService) CreateDirectory(path string) (bool, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func (s *FsService) DeleteDirectory(path string) (bool, error) {
	if err := os.RemoveAll(path); err != nil {
		return false, err
	}
	return true, nil
}

// SearchFiles serves /api/fs/search. When the fff C library is available the
// fuzzy search runs through its index (much faster, frecency-ranked); until
// the index is warm — and always when fff is absent — it falls back to a
// substring walk, mirroring the TS fff-node semantics. Items use the TS
// FileSearchResult shape (relative/absolute paths plus score).
func (s *FsService) SearchFiles(root, query string, limit int, includeDirs bool) ([]types.FileSearchResult, error) {
	if manager != nil && manager.Enabled() {
		if items, ok := manager.SearchAsync(root, query, limit); ok {
			resolved, _ := filepath.Abs(root)
			out := make([]types.FileSearchResult, 0, len(items))
			for _, item := range items {
				full := filepath.Join(resolved, item.RelPath)
				out = append(out, types.FileSearchResult{
					RelativePath: item.RelPath, AbsolutePath: full, IsDir: item.IsDir,
				})
			}
			return out, nil
		}
	}
	resolved, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	q := strings.ToLower(query)
	out := make([]types.FileSearchResult, 0)
	addItem := func(abs string, isDir bool) bool {
		rel, err := filepath.Rel(resolved, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, types.FileSearchResult{RelativePath: rel, AbsolutePath: abs, IsDir: isDir})
		return len(out) >= limit
	}
	_ = filepath.WalkDir(resolved, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != resolved && (utils.IsPathIgnored(d.Name()) || (!includeDirs && false)) {
				return filepath.SkipDir
			}
			if utils.IsHiddenName(d.Name()) && path != resolved {
				return filepath.SkipDir
			}
			if includeDirs && q != "" && strings.Contains(strings.ToLower(d.Name()), q) && path != resolved {
				if addItem(path, true) {
					return filepath.SkipAll
				}
			}
			return nil
		}
		if utils.IsPathIgnored(d.Name()) {
			return nil
		}
		if q != "" {
			// Separator-insensitive comparison so fallback still matches
			// camel/underscore queries like "ptymanager" -> pty_manager.go.
			lq, lp := strings.ToLower(q), strings.ToLower(path[len(resolved)+1:])
			lq = strings.Map(func(r rune) rune {
				if r == '_' || r == '-' || r == '/' || r == '.' {
					return -1
				}
				return r
			}, lq)
			lp = strings.Map(func(r rune) rune {
				if r == '_' || r == '-' || r == '/' || r == '.' {
					return -1
				}
				return r
			}, lp)
			if !strings.Contains(lp, lq) {
				return nil
			}
		}
		if addItem(path, false) {
			return filepath.SkipAll
		}
		return nil
	})
	return out, nil
}

// GrepOptions carries the /api/fs/grep query params, mirroring the global
// search panel's Aa/ab/.* toggles.
type GrepOptions struct {
	Mode         fff.GrepMode
	Case         fff.CaseMode
	WholeWord    bool
	ContextLines int
	MaxMatches   int
	Cursor       uint32
}

// ErrFffUnavailable is returned when the fff index isn't wired in for this
// server (e.g. missing native library), so the caller can render an empty
// state instead of a hard error.
var ErrFffUnavailable = fmt.Errorf("content search index is unavailable")

// Grep serves /api/fs/grep — content search across a workspace root for the
// global search panel (⌘⇧F). Requires the fff index; there is no filesystem
// walk fallback here since the panel is a fast/interactive UI, not the
// agent's best-effort tool.
func (s *FsService) Grep(root, query string, opts GrepOptions) (types.GrepResult, error) {
	if manager == nil || !manager.Enabled() {
		return types.GrepResult{}, ErrFffUnavailable
	}
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return types.GrepResult{}, err
	}
	lease, err := manager.GetOrCreate(resolvedRoot)
	if err != nil {
		return types.GrepResult{}, err
	}
	defer lease.Release()

	result, err := lease.GrepWithOptions(query, fff.GrepOptions{
		Mode:         opts.Mode,
		Case:         opts.Case,
		WholeWord:    opts.WholeWord,
		ContextLines: opts.ContextLines,
		MaxMatches:   opts.MaxMatches,
		Cursor:       opts.Cursor,
	})
	if err != nil {
		return types.GrepResult{}, err
	}

	matches := make([]types.GrepMatch, 0, len(result.Matches))
	for _, m := range result.Matches {
		ranges := make([]types.GrepMatchRange, 0, len(m.MatchRanges))
		for _, r := range m.MatchRanges {
			ranges = append(ranges, types.GrepMatchRange{Start: r.Start, End: r.End})
		}
		matches = append(matches, types.GrepMatch{
			RelPath:      m.RelPath,
			FileName:     m.FileName,
			LineNumber:   m.LineNumber,
			Column:       m.Column,
			EndColumn:    m.EndColumn,
			LineContent:  m.LineContent,
			MatchRanges:  ranges,
			IsBinary:     m.IsBinary,
			IsDefinition: m.IsDefinition,
		})
	}

	out := types.GrepResult{
		Matches:       matches,
		TotalMatched:  result.TotalMatched,
		FilesSearched: result.FilesSearched,
		TotalFiles:    result.TotalFiles,
		FilteredFiles: result.FilteredFiles,
		NextCursor:    result.NextCursor,
		HasMore:       result.HasMore,
	}
	if result.RegexError != nil {
		out.RegexError = result.RegexError.Message
	}
	return out, nil
}
