// The conversation around a pull request: older top-level comments and the
// inline review threads on its files. Both go through the shared 30s cache
// and are dropped by any write to the PR (see invalidate).
package pullrequest

import (
	"context"
	"fmt"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// Comments returns one page of top-level discussion comments, oldest first.
// An empty before is the newest page; pass the response's cursor back to load
// the page older than it while has_more is true.
func (s *Service) Comments(ctx context.Context, owner, repo string, number uint32, before string) (*consolev1.PullRequestCommentsResponse, error) {
	key := fmt.Sprintf("comments|%s/%s|%d|%s", owner, repo, number, before)
	value, err := s.cachedAny(key, func() (any, error) {
		token, err := prToken()
		if err != nil {
			return nil, err
		}
		page, err := github.FetchPRComments(ctx, token, owner, repo, number, before, 30)
		if err != nil {
			return nil, err
		}
		return prCommentsToProto(page), nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*consolev1.PullRequestCommentsResponse), nil
}

// Threads returns the inline review threads, the conversations on lines of
// the changed files. A non-empty path narrows them to one file, so a diff
// viewer can load just the file it is showing.
func (s *Service) Threads(ctx context.Context, owner, repo string, number uint32, path string) (*consolev1.PullRequestThreadsResponse, error) {
	all, err := s.threadsFull(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	out := &consolev1.PullRequestThreadsResponse{}
	for _, t := range all {
		if path == "" || t.Path == path {
			out.Threads = append(out.Threads, prThreadToProto(t))
		}
	}
	return out, nil
}

func (s *Service) threadsFull(ctx context.Context, owner, repo string, number uint32) ([]github.ThreadData, error) {
	key := fmt.Sprintf("threads|%s/%s|%d", owner, repo, number)
	value, err := s.cachedAny(key, func() (any, error) {
		token, err := prToken()
		if err != nil {
			return nil, err
		}
		return github.FetchPRThreads(ctx, token, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return value.([]github.ThreadData), nil
}

func prCommentsToProto(page *github.CommentsPage) *consolev1.PullRequestCommentsResponse {
	out := &consolev1.PullRequestCommentsResponse{Cursor: page.Cursor, HasMore: page.HasMore}
	for _, c := range page.Comments {
		out.Comments = append(out.Comments, &consolev1.PullRequestComment{Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt})
	}
	return out
}

func prThreadToProto(t github.ThreadData) *consolev1.PullRequestThread {
	out := &consolev1.PullRequestThread{
		Id: t.ID, Path: t.Path, Side: t.Side,
		IsResolved: t.IsResolved, IsOutdated: t.IsOutdated,
	}
	if t.HasLine {
		line := t.Line
		out.Line = &line
	}
	if t.HasStart {
		start := t.StartLine
		out.StartLine = &start
	}
	for _, c := range t.Comments {
		out.Comments = append(out.Comments, &consolev1.PullRequestThreadComment{Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt})
	}
	return out
}
