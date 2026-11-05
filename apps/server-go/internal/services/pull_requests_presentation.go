// Presentation fields for pull requests, computed once on the server
// (docs/plan/pull-requests.md §2.5): checks_state, merge_state,
// merge_blocker, status_label. No client or agent re-derives any of it —
// the apps only render, and the agent is told *why* a merge cannot proceed
// rather than retrying.
package services

import (
	"fmt"
	"strings"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// PRChecksState rolls the head commit's checks up to passing | failing |
// pending | none. A failing check beats a pending one; an empty rollup is
// none.
func PRChecksState(rollup string, checks []github.CheckData) string {
	if len(checks) == 0 {
		return prRollupState(rollup)
	}
	anyPassing, anyFailing, anyPending := false, false, false
	for _, c := range checks {
		switch classifyCheck(c) {
		case "passing":
			anyPassing = true
		case "failing":
			anyFailing = true
		default:
			anyPending = true
		}
	}
	switch {
	case anyFailing:
		return "failing"
	case anyPending:
		return "pending"
	case anyPassing:
		return "passing"
	default:
		return "none"
	}
}

// classifyCheck reads one check run or status context as passing, failing,
// or still pending. Skipped and neutral conclusions do not block, so they
// count as passing.
func classifyCheck(c github.CheckData) string {
	switch c.Conclusion {
	case "success", "skipped", "neutral":
		return "passing"
	case "failure", "error", "cancelled", "timed_out", "action_required", "startup_failure":
		return "failing"
	}
	switch c.Status {
	case "success":
		return "passing"
	case "failure", "error":
		return "failing"
	default:
		return "pending"
	}
}

// prRollupState maps the GraphQL statusCheckRollup state enum.
func prRollupState(state string) string {
	switch state {
	case "SUCCESS":
		return "passing"
	case "FAILURE", "ERROR":
		return "failing"
	case "PENDING", "EXPECTED":
		return "pending"
	default:
		return "none"
	}
}

// PRMergeState reads GitHub's mergeability into the server's vocabulary and
// pairs it with the one-line blocker reason.
func PRMergeState(detail *github.DetailData, checksState string) (state, blocker string) {
	if detail.Summary.IsDraft {
		return "draft", "Draft — mark ready for review first."
	}
	if detail.Mergeable == "UNKNOWN" {
		return "checking", ""
	}
	switch detail.MergeStateStatus {
	case "BEHIND":
		return "behind", "The branch is behind the base branch."
	case "DIRTY":
		return "conflicts", "This branch has merge conflicts with the base branch."
	case "BLOCKED":
		return "blocked", blockedReason(detail, checksState)
	default:
		// CLEAN, UNSTABLE (failing non-required checks), HAS_HOOKS: GitHub
		// reports the PR as mergeable.
		return "ready", ""
	}
}

// blockedReason picks the most actionable one-line reason for a
// branch-protection BLOCKED state.
func blockedReason(detail *github.DetailData, checksState string) string {
	failing := 0
	for _, c := range detail.Checks {
		if classifyCheck(c) == "failing" {
			failing++
		}
	}
	if failing > 0 {
		return fmt.Sprintf("%d check%s failing.", failing, plural(failing))
	}
	if checksState == "pending" {
		return "Checks are still running."
	}
	for _, r := range detail.Reviews {
		if r.State == "CHANGES_REQUESTED" {
			return "Changes were requested."
		}
	}
	if detail.Summary.ReviewDecision == "REVIEW_REQUIRED" {
		return "Review is required."
	}
	return "Branch protection is blocking this merge."
}

// PRStatusLabel is the row badge: the single most useful thing to know about
// a PR without opening it.
func PRStatusLabel(summary *github.SummaryData) string {
	switch summary.State {
	case "merged":
		return "Merged"
	case "closed":
		return "Closed"
	}
	if summary.IsDraft {
		return "Draft"
	}
	switch summary.ReviewDecision {
	case "CHANGES_REQUESTED":
		return "Changes requested"
	case "APPROVED":
		return "Approved"
	case "REVIEW_REQUIRED":
		return "Review required"
	}
	switch prRollupState(summary.RollupState) {
	case "failing":
		return "Checks failing"
	case "pending":
		return "Checks pending"
	}
	return "Ready"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// prSummaryToProto maps a GitHub row to the trimmed wire type.
func prSummaryToProto(row github.SummaryData) *consolev1.PullRequestSummary {
	out := &consolev1.PullRequestSummary{
		Number:         row.Number,
		Title:          row.Title,
		State:          row.State,
		IsDraft:        row.IsDraft,
		Author:         row.Author,
		UpdatedAt:      row.UpdatedAt,
		HeadRefName:    row.HeadRefName,
		BaseRefName:    row.BaseRefName,
		ReviewDecision: strings.ToLower(row.ReviewDecision),
		ChecksState:    prRollupState(row.RollupState),
		ChangedFiles:   row.ChangedFiles,
		StatusLabel:    PRStatusLabel(&row),
		RepoOwner:      row.Owner,
		RepoName:       row.Repo,
		Url:            row.URL,
	}
	if row.HasAdditions {
		additions := row.Additions
		out.Additions = &additions
	}
	if row.HasDeletions {
		deletions := row.Deletions
		out.Deletions = &deletions
	}
	return out
}

// prDetailToProto maps a full read, computing merge readiness once.
func prDetailToProto(detail *github.DetailData, methods []string) *consolev1.PullRequestDetail {
	checksState := PRChecksState(detail.Summary.RollupState, detail.Checks)
	state, blocker := PRMergeState(detail, checksState)
	out := &consolev1.PullRequestDetail{
		Summary:             prSummaryToProto(detail.Summary),
		Body:                detail.Body,
		HeadRefOid:          detail.HeadRefOid,
		MergeState:          state,
		MergeBlocker:        blocker,
		AllowedMergeMethods: methods,
		CommentsCursor:      detail.CommentsCursor,
		CommentsHasMore:     detail.CommentsHasMore,
	}
	for _, c := range detail.Checks {
		out.Checks = append(out.Checks, &consolev1.PullRequestCheck{
			Name: c.Name, Status: c.Status, Conclusion: c.Conclusion, Url: c.URL,
		})
	}
	for _, r := range detail.Reviews {
		out.Reviews = append(out.Reviews, &consolev1.PullRequestReview{
			Author: r.Author, State: r.State, SubmittedAt: r.SubmittedAt,
		})
	}
	for _, c := range detail.Comments {
		out.Comments = append(out.Comments, &consolev1.PullRequestComment{
			Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt,
		})
	}
	return out
}

// unifiedDiff turns GitHub's patch field (hunk headers only, no file header)
// into full unified-diff text for the existing diff viewers.
func unifiedDiff(filename, patch string) string {
	var b strings.Builder
	b.WriteString("diff --git a/" + filename + " b/" + filename + "\n")
	b.WriteString("--- a/" + filename + "\n")
	b.WriteString("+++ b/" + filename + "\n")
	b.WriteString(patch)
	if !strings.HasSuffix(patch, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}
