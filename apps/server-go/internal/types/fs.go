// Filesystem types ported from packages/types/src/api.ts and fs.ts.
package types

type FsTreeEntry struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	IsDir     bool          `json:"isDir"`
	Size      *int64        `json:"size,omitempty"`
	GitStatus *string       `json:"gitStatus,omitempty"`
	Children  []FsTreeEntry `json:"children,omitempty"`
}

type FsBrowseResult struct {
	CurrentPath string        `json:"currentPath"`
	ParentPath  *string       `json:"parentPath"`
	Entries     []FsTreeEntry `json:"entries"`
}

// FileSearchResult mirrors the TS assist search item: the desktop ⌘P
// palette requires all four keys (score included).
type FileSearchResult struct {
	RelativePath string  `json:"relativePath"`
	AbsolutePath string  `json:"absolutePath"`
	IsDir        bool    `json:"isDir"`
	Score        float64 `json:"score"`
}

type FsChangeEvent struct {
	Type        string `json:"type"` // "fsChange"
	ProjectPath string `json:"projectPath"`
	EventPath   string `json:"eventPath,omitempty"`
}

type FilePreviewBlock struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// GrepMatchRange is a half-open byte range within GrepMatch.LineContent,
// used by the UI to bold the matched substring in place.
type GrepMatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// GrepMatch is the minimal content-search hit needed by the global search
// panel: identify and open the file, show its line, and highlight the match.
type GrepMatch struct {
	RelPath     string           `json:"relPath"`
	LineNumber  uint64           `json:"lineNumber"`
	LineContent string           `json:"lineContent"`
	MatchRanges []GrepMatchRange `json:"matchRanges"`
}

// GrepResult is the /api/fs/grep response payload. Scanner/index statistics
// stay inside the fff adapter and agent tool; the HTTP client only needs the
// displayed totals, page state, and any regex fallback warning.
type GrepResult struct {
	Matches       []GrepMatch `json:"matches"`
	TotalMatched  int         `json:"totalMatched"`
	FilteredFiles int         `json:"filteredFiles"`
	NextCursor    uint32      `json:"nextCursor"`
	HasMore       bool        `json:"hasMore"`
	RegexError    string      `json:"regexError,omitempty"`
}
