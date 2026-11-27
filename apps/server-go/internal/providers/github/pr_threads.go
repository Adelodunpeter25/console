// GraphQL reads for the conversation around a pull request beyond the first
// page the detail carries: older top-level comments, and inline review
// threads on the changed files.
package github

import (
	"context"
	"fmt"
)

// FetchPRComments returns one page of top-level comments, oldest first. With
// an empty before it is the newest page; pass the previous page's cursor to
// walk back toward the start of the discussion.
func FetchPRComments(ctx context.Context, token, owner, repo string, number uint32, before string, limit int) (*CommentsPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var out struct {
		Repository struct {
			PullRequest *struct {
				Comments struct {
					Nodes []struct {
						Author    *struct{ Login string }
						Body      string
						CreatedAt string
					}
					PageInfo struct {
						StartCursor     string
						HasPreviousPage bool
					}
				}
			}
		}
	}
	query := fmt.Sprintf(`query($owner: String!, $name: String!, $number: Int!, $before: String) {
		repository(owner: $owner, name: $name) {
			pullRequest(number: $number) {
				comments(last: %d, before: $before) {
					nodes { author { login } body createdAt }
					pageInfo { startCursor hasPreviousPage }
				}
			}
		}
	}`, limit)
	vars := map[string]any{"owner": owner, "name": repo, "number": number, "before": nil}
	if before != "" {
		vars["before"] = before
	}
	if err := graphqlRequest(ctx, token, query, vars, &out); err != nil {
		return nil, err
	}
	pr := out.Repository.PullRequest
	if pr == nil {
		return nil, ErrNotFound
	}
	page := &CommentsPage{Cursor: pr.Comments.PageInfo.StartCursor, HasMore: pr.Comments.PageInfo.HasPreviousPage}
	for _, c := range pr.Comments.Nodes {
		page.Comments = append(page.Comments, CommentData{Author: loginOf(c.Author), Body: c.Body, CreatedAt: c.CreatedAt})
	}
	return page, nil
}

// FetchPRThreads returns the inline review threads on a pull request: the
// conversations on specific lines of its files, replies included. Resolved
// and outdated threads are returned and flagged; clients decide what to show.
func FetchPRThreads(ctx context.Context, token, owner, repo string, number uint32) ([]ThreadData, error) {
	var out struct {
		Repository struct {
			PullRequest *struct {
				ReviewThreads struct {
					Nodes []struct {
						ID         string
						Path       string
						Line       *uint32
						StartLine  *uint32
						DiffSide   string
						IsResolved bool
						IsOutdated bool
						Comments   struct {
							Nodes []struct {
								Author    *struct{ Login string }
								Body      string
								CreatedAt string
							}
						}
					}
					PageInfo struct {
						EndCursor   string
						HasNextPage bool
					}
				}
			}
		}
	}
	query := `query($owner: String!, $name: String!, $number: Int!, $after: String) {
		repository(owner: $owner, name: $name) {
			pullRequest(number: $number) {
				reviewThreads(first: 100, after: $after) {
					nodes {
						id path line startLine diffSide isResolved isOutdated
						comments(first: 100) { nodes { author { login } body createdAt } }
					}
					pageInfo { endCursor hasNextPage }
				}
			}
		}
	}`
	var threads []ThreadData
	after := ""
	// 10 pages of 100 threads is far past any real review; the cap keeps a
	// pathological PR from looping forever.
	for page := 0; page < 10; page++ {
		vars := map[string]any{"owner": owner, "name": repo, "number": number, "after": nil}
		if after != "" {
			vars["after"] = after
		}
		out.Repository.PullRequest = nil
		if err := graphqlRequest(ctx, token, query, vars, &out); err != nil {
			return nil, err
		}
		pr := out.Repository.PullRequest
		if pr == nil {
			return nil, ErrNotFound
		}
		for _, n := range pr.ReviewThreads.Nodes {
			t := ThreadData{ID: n.ID, Path: n.Path, Side: n.DiffSide, IsResolved: n.IsResolved, IsOutdated: n.IsOutdated}
			if n.Line != nil {
				t.Line, t.HasLine = *n.Line, true
			}
			if n.StartLine != nil {
				t.StartLine, t.HasStart = *n.StartLine, true
			}
			for _, c := range n.Comments.Nodes {
				t.Comments = append(t.Comments, ThreadCommentData{Author: loginOf(c.Author), Body: c.Body, CreatedAt: c.CreatedAt})
			}
			threads = append(threads, t)
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			break
		}
		after = pr.ReviewThreads.PageInfo.EndCursor
	}
	return threads, nil
}
