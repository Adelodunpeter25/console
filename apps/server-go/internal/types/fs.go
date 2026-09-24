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

// GrepMatch is one content-search hit, shaped for the global search panel
// (grouping by RelPath/FileName, highlighting via MatchRanges).
type GrepMatch struct {
	RelPath      string           `json:"relPath"`
	FileName     string           `json:"fileName"`
	LineNumber   uint64           `json:"lineNumber"`
	Column       int              `json:"column"`
	EndColumn    int              `json:"endColumn"`
	LineContent  string           `json:"lineContent"`
	MatchRanges  []GrepMatchRange `json:"matchRanges"`
	IsBinary     bool             `json:"isBinary,omitempty"`
	IsDefinition bool             `json:"isDefinition,omitempty"`
}

// GrepResult is the /api/fs/grep response payload.
type GrepResult struct {
	Matches       []GrepMatch `json:"matches"`
	TotalMatched  int         `json:"totalMatched"`
	FilesSearched int         `json:"filesSearched"`
	TotalFiles    int         `json:"totalFiles"`
	FilteredFiles int         `json:"filteredFiles"`
	NextCursor    uint32      `json:"nextCursor"`
	HasMore       bool        `json:"hasMore"`
	RegexError    string      `json:"regexError,omitempty"`
}
