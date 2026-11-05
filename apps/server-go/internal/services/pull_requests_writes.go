// Pull-request write actions, served over HTTP now and by the console.pr.*
// agent methods. The service enforces the safety rules once, for both
// transports: a merge always carries the head SHA the caller last read and
// is refused on a draft; creating never implies merging; every write
// invalidates the cached reads it makes stale.
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// Create opens a pull request. Repo resolution: explicit owner/repo, or the
// project's origin when project_id is given. Draft defaults to true — a PR
// starts as a draft until marked ready. Merging is never implied.
func (s *PullRequestService) Create(ctx context.Context, req *consolev1.CreatePullRequestRequest) (*consolev1.CreatePullRequestResponse, error) {
	owner, repo := req.GetOwner(), req.GetRepo()
	if owner == "" || repo == "" {
		if req.GetProjectId() == "" {
			return nil, errors.New("Give owner and repo, or a project_id to resolve them from.")
		}
		var err error
		if owner, repo, err = s.ResolveRepo(req.GetProjectId()); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(req.GetTitle()) == "" || strings.TrimSpace(req.GetHead()) == "" || strings.TrimSpace(req.GetBase()) == "" {
		return nil, errors.New("title, head and base are required.")
	}

	token, err := prToken()
	if err != nil {
		return nil, err
	}
	created, err := github.CreatePullRequest(ctx, token, github.CreatePRInput{
		Owner: owner, Repo: repo,
		Title: req.GetTitle(), Body: req.GetBody(),
		Head: req.GetHead(), Base: req.GetBase(),
		// Unset draft reads as true: PRs are drafts until marked ready.
		Draft: req.Draft == nil || req.GetDraft(),
	})
	if err != nil {
		return nil, err
	}
	s.invalidate(owner, repo)
	return &consolev1.CreatePullRequestResponse{Number: created.Number, Url: created.URL}, nil
}

// Update patches title/body/base, changes state (open/closed), or flips
// ready/draft — whichever fields are set. The refreshed detail comes back so
// a client re-renders from one response.
func (s *PullRequestService) Update(ctx context.Context, req *consolev1.UpdatePullRequestRequest) (*consolev1.UpdatePullRequestResponse, error) {
	token, err := prToken()
	if err != nil {
		return nil, err
	}
	owner, repo, number := req.GetOwner(), req.GetRepo(), req.GetNumber()
	if number == 0 {
		return nil, errors.New("Pull request number is required.")
	}

	// Ready/draft: GraphQL, on the node id from a fresh read.
	if req.Draft != nil {
		raw, err := github.FetchPRDetail(ctx, token, owner, repo, number)
		if err != nil {
			return nil, err
		}
		if err := github.SetPullRequestDraft(ctx, token, raw.ID, req.GetDraft()); err != nil {
			return nil, err
		}
	}
	// Everything else: one PATCH carrying only the set fields.
	if err := github.UpdatePullRequest(ctx, token, owner, repo, number, github.UpdatePRFields{
		Title: req.Title, Body: req.Body, Base: req.Base, State: req.GetState(),
	}); err != nil {
		return nil, err
	}

	s.invalidate(owner, repo)
	detail, err := s.Detail(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	return &consolev1.UpdatePullRequestResponse{Detail: detail}, nil
}

// Merge merges with a method the repository allows, refusing a draft, and
// carrying expected_head so GitHub refuses a moved branch (409) instead of
// landing code nobody reviewed.
func (s *PullRequestService) Merge(ctx context.Context, req *consolev1.MergePullRequestRequest) (*consolev1.MergePullRequestResponse, error) {
	owner, repo, number := req.GetOwner(), req.GetRepo(), req.GetNumber()
	if number == 0 {
		return nil, errors.New("Pull request number is required.")
	}
	if strings.TrimSpace(req.GetExpectedHead()) == "" {
		return nil, errors.New("expected_head is required: pass the head SHA from the last read so a moved branch is refused.")
	}
	method := req.GetMethod()
	if method == "" {
		method = "merge"
	}
	if method != "merge" && method != "squash" && method != "rebase" {
		return nil, fmt.Errorf("Merge method must be merge, squash or rebase, not %q.", method)
	}

	token, err := prToken()
	if err != nil {
		return nil, err
	}

	// A draft cannot merge: refuse here, with the same one-line blocker the
	// UIs show, instead of relaying whatever GitHub says.
	detail, err := s.Detail(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	if detail.Summary.IsDraft {
		return nil, errors.New(detail.MergeBlocker)
	}
	// Reject a method the repository disallows before the request, so the
	// client never offers one.
	if !mergeMethodAllowed(detail.AllowedMergeMethods, method) {
		return nil, fmt.Errorf("This repository does not allow the %q merge method. Allowed: %s.",
			method, strings.Join(detail.AllowedMergeMethods, ", "))
	}

	merged, err := github.MergePullRequest(ctx, token, owner, repo, number, method, req.GetExpectedHead())
	if err != nil {
		return nil, err
	}
	s.invalidate(owner, repo)
	return &consolev1.MergePullRequestResponse{MergedSha: merged.SHA, Method: method}, nil
}

// Comment adds one top-level comment and returns it.
func (s *PullRequestService) Comment(ctx context.Context, req *consolev1.CreateCommentRequest) (*consolev1.CreateCommentResponse, error) {
	if strings.TrimSpace(req.GetBody()) == "" {
		return nil, errors.New("Comment body is empty.")
	}
	token, err := prToken()
	if err != nil {
		return nil, err
	}
	created, err := github.CreatePRComment(ctx, token, req.GetOwner(), req.GetRepo(), req.GetNumber(), req.GetBody())
	if err != nil {
		return nil, err
	}
	s.invalidate(req.GetOwner(), req.GetRepo())
	return &consolev1.CreateCommentResponse{Comment: &consolev1.PullRequestComment{
		Author: created.Author, Body: created.Body, CreatedAt: created.CreatedAt,
	}}, nil
}

// mergeMethodAllowed reports whether the repository allows the method.
func mergeMethodAllowed(allowed []string, method string) bool {
	for _, a := range allowed {
		if a == method {
			return true
		}
	}
	return false
}

// invalidate drops the cached reads a write makes stale: this PR's detail
// and files, and every list (the cross-repo scopes span all repos, so a
// targeted invalidation cannot know which lists this PR appears in).
func (s *PullRequestService) invalidate(owner, repo string) {
	prefix := fmt.Sprintf("%s/%s|", owner, repo)
	s.mu.Lock()
	for key := range s.cache {
		if strings.HasPrefix(key, "list|") ||
			strings.HasPrefix(key, "detail|"+prefix) ||
			strings.HasPrefix(key, "files|"+prefix) {
			delete(s.cache, key)
		}
	}
	s.mu.Unlock()
}
