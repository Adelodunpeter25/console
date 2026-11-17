// GraphQL reads for pull requests: one request for a list, one for a
// detail. "Mine" and "review requested" span
// every repo, so they run through GitHub search; a repo's list is a single
// repository.pullRequests query. Selections are trimmed to what screens
// draw — no bodies, no files, no comments in the list.
package github

import (
	"context"
	"fmt"
	"strings"
)

// prSummaryFields is the GraphQL selection shared by list rows and the
// detail query.
const prSummaryFields = `
	number title state isDraft
	author { login }
	updatedAt
	headRefName baseRefName
	reviewDecision
	additions deletions changedFiles
	url
	repository { owner { login } name }
	commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
`

// ListMine returns PRs the token's account authored, across every repo.
func ListMine(ctx context.Context, token, state, draft string) ([]SummaryData, error) {
	return searchList(ctx, token, "author:@me "+stateQualifier(state)+draftQualifier(draft))
}

// ListReviewRequested returns PRs where the account's review is requested.
func ListReviewRequested(ctx context.Context, token, state, draft string) ([]SummaryData, error) {
	return searchList(ctx, token, "review-requested:@me "+stateQualifier(state)+draftQualifier(draft))
}

// searchList fetches one page of a cross-repo PR search. GitHub's search
// API is the only way to span repos; it is also the most expensive query,
// so callers cache and never retry.
func searchList(ctx context.Context, token, qualifiers string) ([]SummaryData, error) {
	var out struct {
		Search struct {
			Nodes []struct {
				Number         uint32
				Title          string
				State          string
				IsDraft        bool
				Author         *struct{ Login string }
				UpdatedAt      string
				HeadRefName    string
				BaseRefName    string
				ReviewDecision *string
				Additions      *uint32
				Deletions      *uint32
				ChangedFiles   uint32
				URL            string
				Repository     struct {
					Owner struct{ Login string }
					Name  string
				}
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct{ State string }
						}
					}
				}
			} `json:"nodes"`
		} `json:"search"`
	}
	query := "is:pr sort:updated-desc " + qualifiers
	if err := graphqlRequest(ctx, token, query, map[string]any{}, &out); err != nil {
		return nil, err
	}
	rows := make([]SummaryData, 0, len(out.Search.Nodes))
	for _, n := range out.Search.Nodes {
		row := SummaryData{
			Number: n.Number, Title: n.Title, State: lower(n.State), IsDraft: n.IsDraft,
			Author: loginOf(n.Author), UpdatedAt: n.UpdatedAt,
			HeadRefName: n.HeadRefName, BaseRefName: n.BaseRefName,
			ChangedFiles: n.ChangedFiles, URL: n.URL,
			Owner: n.Repository.Owner.Login, Repo: n.Repository.Name,
		}
		if n.ReviewDecision != nil {
			row.ReviewDecision = *n.ReviewDecision
		}
		if n.Additions != nil {
			row.Additions, row.HasAdditions = *n.Additions, true
		}
		if n.Deletions != nil {
			row.Deletions, row.HasDeletions = *n.Deletions, true
		}
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			row.RollupState = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ListRepo returns a repository's open PRs (single GraphQL query, no
// search). The repository query has no draft filter; callers apply one.
func ListRepo(ctx context.Context, token, owner, repo, state, draft string) ([]SummaryData, error) {
	var out struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number         uint32
					Title          string
					State          string
					IsDraft        bool
					Author         *struct{ Login string }
					UpdatedAt      string
					HeadRefName    string
					BaseRefName    string
					ReviewDecision *string
					Additions      *uint32
					Deletions      *uint32
					ChangedFiles   uint32
					URL            string
					Commits        struct {
						Nodes []struct {
							Commit struct {
								StatusCheckRollup *struct{ State string }
							}
						}
					}
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	states := "OPEN"
	switch state {
	case "closed":
		states = "CLOSED"
	case "merged":
		states = "MERGED"
	}
	query := fmt.Sprintf(`query($owner: String!, $name: String!, $states: [PullRequestState!]) {
		repository(owner: $owner, name: $name) {
			pullRequests(states: $states, first: 30, orderBy: {field: UPDATED_AT, direction: DESC}) {
				nodes { %s }
			}
		}
	}`, prSummaryFields)
	if err := graphqlRequest(ctx, token, query, map[string]any{"owner": owner, "name": repo, "states": []string{states}}, &out); err != nil {
		return nil, err
	}
	rows := make([]SummaryData, 0, len(out.Repository.PullRequests.Nodes))
	for _, n := range out.Repository.PullRequests.Nodes {
		row := SummaryData{
			Number: n.Number, Title: n.Title, State: lower(n.State), IsDraft: n.IsDraft,
			Author: loginOf(n.Author), UpdatedAt: n.UpdatedAt,
			HeadRefName: n.HeadRefName, BaseRefName: n.BaseRefName,
			ChangedFiles: n.ChangedFiles, URL: n.URL,
			Owner: owner, Repo: repo,
		}
		if n.ReviewDecision != nil {
			row.ReviewDecision = *n.ReviewDecision
		}
		if n.Additions != nil {
			row.Additions, row.HasAdditions = *n.Additions, true
		}
		if n.Deletions != nil {
			row.Deletions, row.HasDeletions = *n.Deletions, true
		}
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			row.RollupState = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// FetchPRDetail reads one PR's full detail: the summary fields plus body,
// mergeability, check runs, the latest review per reviewer, and the last
// page of top-level comments. Files are fetched by pr_rest.go instead.
func FetchPRDetail(ctx context.Context, token, owner, repo string, number uint32) (*DetailData, error) {
	var out struct {
		Repository struct {
			PullRequest *struct {
				ID               string
				Number           uint32
				Title            string
				State            string
				IsDraft          bool
				Author           *struct{ Login string }
				UpdatedAt        string
				HeadRefName      string
				BaseRefName      string
				ReviewDecision   *string
				Additions        *uint32
				Deletions        *uint32
				ChangedFiles     uint32
				URL              string
				Body             string
				HeadRefOid       string
				Mergeable        string
				MergeStateStatus string
				Repository       struct {
					Owner struct{ Login string }
					Name  string
				}
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State    string
								Contexts struct {
									Nodes []struct {
										TypeName   string `json:"__typename"`
										Name       string
										Status     string
										Conclusion *string
										DetailsUrl string
										TargetUrl  string
										State      string
									} `json:"nodes"`
								}
							}
						}
					} `json:"nodes"`
				}
				Reviews struct {
					Nodes []struct {
						Author      *struct{ Login string }
						State       string
						SubmittedAt string
					} `json:"nodes"`
				} `json:"reviews"`
				Comments struct {
					Nodes []struct {
						Author    *struct{ Login string }
						Body      string
						CreatedAt string
					} `json:"nodes"`
					PageInfo struct {
						EndCursor   string
						HasNextPage bool
					} `json:"pageInfo"`
				} `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	query := fmt.Sprintf(`query($owner: String!, $name: String!, $number: Int!) {
		repository(owner: $owner, name: $name) {
			pullRequest(number: $number) {
				id
				%s
				body
				headRefOid
				mergeable
				mergeStateStatus
				commits(last: 1) { nodes { commit {
					statusCheckRollup {
						state
						contexts(first: 50) { nodes {
							__typename
							... on CheckRun { name status conclusion detailsUrl }
							... on StatusContext { name state targetUrl }
						} }
					}
				} } }
				reviews(last: 100) { nodes { author { login } state submittedAt } }
				comments(last: 30) { nodes { author { login } body createdAt }
					pageInfo { endCursor hasNextPage } }
			}
		}
	}`, prSummaryFields)
	if err := graphqlRequest(ctx, token, query, map[string]any{"owner": owner, "name": repo, "number": number}, &out); err != nil {
		return nil, err
	}
	pr := out.Repository.PullRequest
	if pr == nil {
		return nil, ErrNotFound
	}

	detail := &DetailData{
		ID:               pr.ID,
		Body:             pr.Body,
		HeadRefOid:       pr.HeadRefOid,
		Mergeable:        pr.Mergeable,
		MergeStateStatus: pr.MergeStateStatus,
	}
	detail.Summary = SummaryData{
		Number: pr.Number, Title: pr.Title, State: lower(pr.State), IsDraft: pr.IsDraft,
		Author: loginOf(pr.Author), UpdatedAt: pr.UpdatedAt,
		HeadRefName: pr.HeadRefName, BaseRefName: pr.BaseRefName,
		ChangedFiles: pr.ChangedFiles, URL: pr.URL,
		Owner: pr.Repository.Owner.Login, Repo: pr.Repository.Name,
	}
	if pr.ReviewDecision != nil {
		detail.Summary.ReviewDecision = *pr.ReviewDecision
	}
	if pr.Additions != nil {
		detail.Summary.Additions, detail.Summary.HasAdditions = *pr.Additions, true
	}
	if pr.Deletions != nil {
		detail.Summary.Deletions, detail.Summary.HasDeletions = *pr.Deletions, true
	}
	if len(pr.Commits.Nodes) > 0 {
		rollup := pr.Commits.Nodes[0].Commit.StatusCheckRollup
		if rollup != nil {
			detail.Summary.RollupState = rollup.State
			for _, c := range rollup.Contexts.Nodes {
				check := CheckData{Name: c.Name, Status: lower(c.Status), URL: c.DetailsUrl}
				if check.URL == "" {
					check.URL = c.TargetUrl
				}
				if c.TypeName == "StatusContext" {
					// Status contexts have no separate status/conclusion: the
					// state is the outcome (success | failure | pending).
					check.Status = lower(c.State)
					check.Conclusion = lower(c.State)
				} else if c.Conclusion != nil {
					check.Conclusion = lower(*c.Conclusion)
				}
				detail.Checks = append(detail.Checks, check)
			}
		}
	}
	for _, r := range pr.Reviews.Nodes {
		detail.Reviews = append(detail.Reviews, ReviewData{
			Author: loginOf(r.Author), State: r.State, SubmittedAt: r.SubmittedAt,
		})
	}
	for _, c := range pr.Comments.Nodes {
		detail.Comments = append(detail.Comments, CommentData{
			Author: loginOf(c.Author), Body: c.Body, CreatedAt: c.CreatedAt,
		})
	}
	detail.CommentsCursor = pr.Comments.PageInfo.EndCursor
	detail.CommentsHasMore = pr.Comments.PageInfo.HasNextPage
	return detail, nil
}

func loginOf(a *struct{ Login string }) string {
	if a == nil {
		return ""
	}
	return a.Login
}

func lower(s string) string { return strings.ToLower(s) }

// stateQualifier maps the API's state filter onto search syntax; default open.
func stateQualifier(state string) string {
	switch state {
	case "closed":
		return "is:closed"
	case "merged":
		return "is:merged"
	default:
		return "is:open"
	}
}

func draftQualifier(draft string) string {
	switch draft {
	case "true":
		return " draft:true"
	case "false":
		return " draft:false"
	default:
		return ""
	}
}
