// REST reads for pull requests: changed files (§2.4) and the repository
// settings that say which merge methods are allowed (§2.3). Simple calls
// where REST has the clearest shapes.
package github

import (
	"context"
	"fmt"
	"net/url"
)

// FetchAllowedMergeMethods reads which merge methods the repository allows
// so clients never offer one GitHub will 405.
func FetchAllowedMergeMethods(ctx context.Context, token, owner, repo string) ([]string, error) {
	var repoInfo struct {
		AllowMergeCommit bool `json:"allow_merge_commit"`
		AllowSquashMerge bool `json:"allow_squash_merge"`
		AllowRebaseMerge bool `json:"allow_rebase_merge"`
	}
	if err := restGet(ctx, token, "/repos/"+owner+"/"+repo, nil, &repoInfo); err != nil {
		return nil, err
	}
	var methods []string
	if repoInfo.AllowMergeCommit {
		methods = append(methods, "merge")
	}
	if repoInfo.AllowSquashMerge {
		methods = append(methods, "squash")
	}
	if repoInfo.AllowRebaseMerge {
		methods = append(methods, "rebase")
	}
	return methods, nil
}

// FetchPRFiles lists a PR's changed files across all pages (GitHub caps PRs
// at 3000 files). Patches are included here; the service strips them for
// the file list and keeps them for single-file diffs.
func FetchPRFiles(ctx context.Context, token, owner, repo string, number uint32) ([]FileData, error) {
	var files []FileData
	for page := 1; page <= 30; page++ {
		var batch []FileData
		q := url.Values{"per_page": {"100"}, "page": {fmt.Sprint(page)}}
		if err := restGet(ctx, token, fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, number), q, &batch); err != nil {
			return nil, err
		}
		files = append(files, batch...)
		if len(batch) < 100 {
			return files, nil
		}
	}
	return files, nil
}
