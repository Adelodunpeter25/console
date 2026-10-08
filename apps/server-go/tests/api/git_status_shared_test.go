// Concurrent status requests for one repo share a computation, and the watch
// stream stops working once its client disconnects.
package tests

import (
	"sync"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestGitStatusConcurrentCallsAgree(t *testing.T) {
	repo := gitRepo(t)
	git := services.NewGitService()
	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = git.GetGitStatus(repo).Branch
		}(i)
	}
	wg.Wait()
	for i, b := range results {
		if b != "main" {
			t.Fatalf("call %d branch %q", i, b)
		}
	}
	// A later call after the shared one finished recomputes (no stale cache).
	gitIn(t, repo, "checkout", "-b", "other")
	if b := git.GetGitStatus(repo).Branch; b != "other" {
		t.Fatalf("stale shared result: %q", b)
	}
}
