// Pull-request service (docs/plan/pull-requests.md §2). GitHub only in v1.
// Owns the token (loaded per call, never cached, never returned), one 30s
// response cache with in-flight de-duplication shaped like
// providers/usage/service.go, and the UNKNOWN-mergeability retry. The
// presentation fields it computes live in pull_requests_presentation.go;
// repo resolution in pull_requests_repo.go.
package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

// prCacheTTL is the short server cache for GitHub reads. A write (phase 2)
// invalidates the affected PR and the lists.
const prCacheTTL = 30 * time.Second

type prCacheEntry struct {
	value     any
	fetchedAt time.Time
}

type prCall struct {
	done  chan struct{}
	value any
	err   error
}

// PullRequestService serves pull-request reads to the routes and (phase 3)
// the console.pr.* agent methods, which call the same service.
type PullRequestService struct {
	projects *projectssvc.ProjectService

	mu       sync.Mutex
	cache    map[string]prCacheEntry
	inflight map[string]*prCall

	// RetryDelay is how long to wait before the single re-read when GitHub
	// still reports mergeability as UNKNOWN. One second in production; tests
	// shorten it.
	RetryDelay time.Duration
}

func NewPullRequestService(projects *projectssvc.ProjectService) *PullRequestService {
	return &PullRequestService{
		projects:   projects,
		cache:      map[string]prCacheEntry{},
		inflight:   map[string]*prCall{},
		RetryDelay: time.Second,
	}
}

// List returns trimmed PR rows. scope is mine | review | repo; repo requires
// project_id and resolves it to the project's origin remote.
func (s *PullRequestService) List(ctx context.Context, scope, state, projectID, draft string) (*consolev1.PullRequestListResponse, error) {
	var owner, repo string
	switch scope {
	case "mine", "review":
	case "repo":
		var err error
		if owner, repo, err = s.ResolveRepo(projectID); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("scope must be one of mine, review, repo.")
	}

	value, err := s.cachedAny(prListKey(scope, state, draft, projectID, owner+"/"+repo), func() (any, error) {
		return s.listUncached(ctx, scope, state, draft, owner, repo)
	})
	if err != nil {
		return nil, err
	}
	return value.(*consolev1.PullRequestListResponse), nil
}

func (s *PullRequestService) listUncached(ctx context.Context, scope, state, draft, owner, repo string) (*consolev1.PullRequestListResponse, error) {
	token, err := prToken()
	if err != nil {
		return nil, err
	}
	var rows []github.SummaryData
	switch scope {
	case "mine":
		rows, err = github.ListMine(ctx, token, state, draft)
	case "review":
		rows, err = github.ListReviewRequested(ctx, token, state, draft)
	case "repo":
		rows, err = github.ListRepo(ctx, token, owner, repo, state, draft)
		// The repository query has no draft filter; apply it here.
		rows = filterDraft(rows, draft)
	}
	if err != nil {
		return nil, err
	}
	return prListToProto(rows), nil
}

// Detail reads one PR: summary fields, body, checks, reviews, comments, and
// merge readiness. GitHub computes mergeability lazily — the first read
// after a push can come back UNKNOWN — so a checking result is re-read once
// after a short delay and otherwise reported as checking, never mergeable.
func (s *PullRequestService) Detail(ctx context.Context, owner, repo string, number uint32) (*consolev1.PullRequestDetail, error) {
	key := fmt.Sprintf("detail|%s/%s|%d", owner, repo, number)
	value, err := s.cachedAny(key, func() (any, error) {
		return s.detailUncached(ctx, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return value.(*consolev1.PullRequestDetail), nil
}

func (s *PullRequestService) detailUncached(ctx context.Context, owner, repo string, number uint32) (*consolev1.PullRequestDetail, error) {
	token, err := prToken()
	if err != nil {
		return nil, err
	}
	detail, err := github.FetchPRDetail(ctx, token, owner, repo, number)
	if err != nil {
		return nil, err
	}
	methods, err := github.FetchAllowedMergeMethods(ctx, token, owner, repo)
	if err != nil {
		return nil, err
	}

	// Mergeability is computed lazily by GitHub: UNKNOWN right after a push.
	// Re-read once, then report checking rather than guessing.
	if detail.Mergeable == "UNKNOWN" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.RetryDelay):
		}
		reread, err := github.FetchPRDetail(ctx, token, owner, repo, number)
		if err == nil && reread.Mergeable != "UNKNOWN" {
			detail = reread
		}
	}

	out := prDetailToProto(detail, methods)
	return out, nil
}

// prIsChecking reports whether a cached value is an interim "checking"
// detail: GitHub has not computed mergeability yet, so it must not be
// cached — the next read should get the verdict instead.
func prIsChecking(value any) bool {
	detail, ok := value.(*consolev1.PullRequestDetail)
	return ok && detail.MergeState == "checking"
}

// Files lists a PR's changed files without patches (paths, status, +/−).
func (s *PullRequestService) Files(ctx context.Context, owner, repo string, number uint32) (*consolev1.PullRequestFilesResponse, error) {
	full, err := s.filesFull(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	out := &consolev1.PullRequestFilesResponse{}
	for _, f := range full {
		additions, deletions := f.Additions, f.Deletions
		out.Files = append(out.Files, &consolev1.PullRequestFile{
			Filename: f.Filename, Status: f.Status,
			Additions: &additions, Deletions: &deletions,
		})
	}
	return out, nil
}

// FileDiff converts one changed file's GitHub patch into the unified-diff
// text the existing desktop and Android diff viewers render unchanged.
func (s *PullRequestService) FileDiff(ctx context.Context, owner, repo string, number uint32, path string) (*consolev1.PullRequestFileDiffResponse, error) {
	full, err := s.filesFull(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	for _, fd := range full {
		if fd.Filename != path {
			continue
		}
		if strings.TrimSpace(fd.Patch) == "" {
			// GitHub omits patches for very large or binary files.
			return &consolev1.PullRequestFileDiffResponse{Filename: path, TooLarge: true, Url: prURL}, nil
		}
		return &consolev1.PullRequestFileDiffResponse{
			Filename: path,
			Diff:     unifiedDiff(fd.Filename, fd.Patch),
			Url:      prURL,
		}, nil
	}
	return nil, fmt.Errorf("File %q is not part of this pull request.", path)
}

// filesFull fetches every changed file, patches included, through the shared
// cache. Patches stay server-side: the file list strips them, and a single
// file's patch leaves only when that file is opened.
func (s *PullRequestService) filesFull(ctx context.Context, owner, repo string, number uint32) ([]github.FileData, error) {
	key := fmt.Sprintf("files|%s/%s|%d", owner, repo, number)
	value, err := s.cachedAny(key, func() (any, error) {
		token, err := prToken()
		if err != nil {
			return nil, err
		}
		return github.FetchPRFiles(ctx, token, owner, repo, number)
	})
	if err != nil {
		return nil, err
	}
	return value.([]github.FileData), nil
}

// cachedAny serves a 30s-cached response, coalescing concurrent identical
// reads into one upstream call (the providers/usage pattern).
func (s *PullRequestService) cachedAny(key string, fetch func() (any, error)) (any, error) {
	s.mu.Lock()
	if entry, ok := s.cache[key]; ok && time.Since(entry.fetchedAt) < prCacheTTL {
		s.mu.Unlock()
		return entry.value, nil
	}
	if call, ok := s.inflight[key]; ok {
		s.mu.Unlock()
		<-call.done
		return call.value, call.err
	}
	call := &prCall{done: make(chan struct{})}
	s.inflight[key] = call
	s.mu.Unlock()

	call.value, call.err = fetch()
	close(call.done)

	s.mu.Lock()
	delete(s.inflight, key)
	// A "checking" detail is deliberately not cached: the next read should
	// get GitHub's verdict instead of the interim answer.
	if call.err == nil && !prIsChecking(call.value) {
		s.cache[key] = prCacheEntry{value: call.value, fetchedAt: time.Now()}
	}
	s.mu.Unlock()
	return call.value, call.err
}

// prToken loads the stored credential for this one call. The token never
// lands in a cache, a log line, or a response.
func prToken() (string, error) {
	cred, err := github.LoadCredential()
	if err != nil || strings.TrimSpace(cred.Token) == "" {
		return "", github.ErrNotConnected
	}
	return strings.TrimSpace(cred.Token), nil
}

// prListKey builds the cache key for a list read; repo is empty outside the
// repo scope.
func prListKey(scope, state, draft, projectID, repo string) string {
	return fmt.Sprintf("list|%s|%s|%s|%s|%s", scope, state, draft, projectID, repo)
}

// filterDraft applies the draft filter the repository query cannot express;
// the search API already folded it into its qualifiers.
func filterDraft(rows []github.SummaryData, draft string) []github.SummaryData {
	switch draft {
	case "true":
		filtered := rows[:0]
		for _, r := range rows {
			if r.IsDraft {
				filtered = append(filtered, r)
			}
		}
		return filtered
	case "false":
		filtered := rows[:0]
		for _, r := range rows {
			if !r.IsDraft {
				filtered = append(filtered, r)
			}
		}
		return filtered
	default:
		return rows
	}
}

// prListToProto maps rows to the trimmed wire type. Kept next to the list
// methods so the two stay in sync.
func prListToProto(rows []github.SummaryData) *consolev1.PullRequestListResponse {
	out := &consolev1.PullRequestListResponse{}
	for _, row := range rows {
		out.PullRequests = append(out.PullRequests, prSummaryToProto(row))
	}
	return out
}
