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
