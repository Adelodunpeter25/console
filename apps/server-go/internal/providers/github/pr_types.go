// Data shapes returned by the pull-request read client. Plain Go structs —
// the proto conversion happens in the service layer, which also computes
// the presentation fields.
package github

// SummaryData is one trimmed list row.
type SummaryData struct {
	Owner          string
	Repo           string
	Number         uint32
	Title          string
	State          string // open | closed | merged
	IsDraft        bool
	Author         string
	UpdatedAt      string
	HeadRefName    string
	BaseRefName    string
	ReviewDecision string
	// RollupState is the head commit's statusCheckRollup state, one of
	// SUCCESS | FAILURE | PENDING | ERROR | EXPECTED; empty when the commit
	// has no checks at all.
	RollupState  string
	Additions    uint32
	HasAdditions bool
	Deletions    uint32
	HasDeletions bool
	ChangedFiles uint32
	URL          string
}

// CheckData is one check run or status context on the head commit.
type CheckData struct {
	Name       string
	Status     string
	Conclusion string
	URL        string
}

// ReviewData is the latest review of one reviewer.
type ReviewData struct {
	Author      string
	State       string
	SubmittedAt string
}

// CommentData is one top-level issue comment.
type CommentData struct {
	Author    string
	Body      string
	CreatedAt string
}

// DetailData is a PR's full read: the summary fields plus everything the
// detail screen draws.
type DetailData struct {
	Summary          SummaryData
	Body             string
	HeadRefOid       string
	Mergeable        string // MERGEABLE | CONFLICTING | UNKNOWN
	MergeStateStatus string // CLEAN | BLOCKED | DIRTY | BEHIND | UNSTABLE | HAS_HOOKS | DRAFT | ...
	Checks           []CheckData
	Reviews          []ReviewData
	Comments         []CommentData
	CommentsCursor   string
	CommentsHasMore  bool
}

// FileData is one changed file. Patch is GitHub's unified-diff hunks without
// the ---/+++ header lines, and is omitted by GitHub for very large or
// binary files.
type FileData struct {
	Filename  string
	Status    string
	Additions uint32
	Deletions uint32
	Patch     string
}
