// Write actions for pull requests: REST where it has the clearest error
// semantics (create, update, state, comment, merge), GraphQL for the two
// transitions REST has no endpoint for (ready for review, convert to
// draft). A merge always carries the head SHA the caller saw, so a branch
// that moved since is refused (409) instead of merged unreviewed.
package github

import (
	"context"
	"fmt"
)

// CreatePRInput is one new pull request.
type CreatePRInput struct {
	Owner string
	Repo  string
	Title string
	Body  string
	Head  string
	Base  string
	Draft bool
}

// CreatedPR is the useful part of a create response.
type CreatedPR struct {
	Number uint32 `json:"number"`
	URL    string `json:"html_url"`
}

// CreatePullRequest opens a pull request. Merging is never implied.
func CreatePullRequest(ctx context.Context, token string, in CreatePRInput) (*CreatedPR, error) {
	var out CreatedPR
	body := map[string]any{
		"title": in.Title, "body": in.Body, "head": in.Head, "base": in.Base, "draft": in.Draft,
	}
	if err := restWrite(ctx, token, "POST", fmt.Sprintf("/repos/%s/%s/pulls", in.Owner, in.Repo), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePRFields carries only the fields to change; nil pointers are left
// untouched.
type UpdatePRFields struct {
	Title *string
	Body  *string
	Base  *string
	// State is "open" or "closed"; empty leaves it unchanged.
	State string
}

// UpdatePullRequest patches title/body/base and/or state.
func UpdatePullRequest(ctx context.Context, token, owner, repo string, number uint32, f UpdatePRFields) error {
	body := map[string]any{}
	if f.Title != nil {
		body["title"] = *f.Title
	}
	if f.Body != nil {
		body["body"] = *f.Body
	}
	if f.Base != nil {
		body["base"] = *f.Base
	}
	if f.State != "" {
		body["state"] = f.State
	}
	if len(body) == 0 {
		return nil
	}
	return restWrite(ctx, token, "PATCH", fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), body, nil)
}

// SetPullRequestDraft marks a PR ready for review (draft=false) or converts
// it back to draft (draft=true). REST has no endpoint for this transition;
// it is a GraphQL mutation on the PR's node id.
func SetPullRequestDraft(ctx context.Context, token, prNodeID string, draft bool) error {
	var payload struct {
		UpdatePullRequest struct {
			PullRequest struct {
				IsDraft bool
			}
		} `json:"updatePullRequest"`
	}
	input := map[string]any{"pullRequestId": prNodeID, "draft": draft}
	return graphqlMutation(ctx, token, `mutation($input: UpdatePullRequestInput!) {
		updatePullRequest(input: $input) { pullRequest { isDraft } }
	}`, input, &payload)
}

// CreatePRComment adds one top-level comment (issue comments carry PR
// conversations; inline review threads are out of scope in v1).
func CreatePRComment(ctx context.Context, token, owner, repo string, number uint32, body string) (*CommentData, error) {
	var out struct {
		User      *struct{ Login string } `json:"user"`
		Body      string                  `json:"body"`
		CreatedAt string                  `json:"created_at"`
	}
	if err := restWrite(ctx, token, "POST", fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, number), map[string]any{"body": body}, &out); err != nil {
		return nil, err
	}
	comment := &CommentData{Body: out.Body, CreatedAt: out.CreatedAt}
	if out.User != nil {
		comment.Author = out.User.Login
	}
	return comment, nil
}

// MergedPR is the useful part of a merge response.
type MergedPR struct {
	SHA string `json:"sha"`
}

// MergePullRequest merges with the given method (merge | squash | rebase),
// refusing unless the branch is still at sha. A 405 means GitHub will not
// merge (failing required checks, missing review) and surfaces its own
// message; a 409 means the head moved since the caller read it.
func MergePullRequest(ctx context.Context, token, owner, repo string, number uint32, method, sha string) (*MergedPR, error) {
	var out MergedPR
	body := map[string]any{"merge_method": method, "sha": sha}
	if err := restWrite(ctx, token, "PUT", fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, number), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// graphqlMutation posts one mutation document and decodes the `data` field.
func graphqlMutation(ctx context.Context, token, mutation string, input map[string]any, out any) error {
	return graphqlRequest(ctx, token, mutation, map[string]any{"input": input}, out)
}
