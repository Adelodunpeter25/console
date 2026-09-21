// Git types ported from packages/types/src/api.ts.
package types

type GitFileStatus string // M, A, D, R, C, U, "?", "!"

type GitFileEntry struct {
	Path      string        `json:"path"`
	Status    GitFileStatus `json:"status"`
	Staged    bool          `json:"staged"`
	Additions int64         `json:"additions"`
	Deletions int64         `json:"deletions"`
}

type GitStatusSummary struct {
	Branch string         `json:"branch"`
	Clean  bool           `json:"clean"`
	Files  []GitFileEntry `json:"files"`
}

type GitBranchInfo struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

type GitBranchesResponse struct {
	Branches        []GitBranchInfo `json:"branches"`
	IsGitRepository bool            `json:"isGitRepository"`
}
