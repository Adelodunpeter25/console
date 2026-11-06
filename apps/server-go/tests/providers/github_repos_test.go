// Coverage for the repo listing behind the pickers: not-connected mapping,
// field mapping from the /user/repos payload, and auth-failure mapping.
package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

func TestListReposNotConnected(t *testing.T) {
	withCredsPath(t)

	if _, err := github.ListRepos(context.Background(), nil); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("err = %v, want ErrNotConnected", err)
	}
}

func TestListReposMapsPayload(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_list_test"}); err != nil {
		t.Fatal(err)
	}

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`[{"full_name":"octocat/hello","name":"hello","private":true,
			"description":"hi","html_url":"https://github.com/octocat/hello",
			"clone_url":"https://github.com/octocat/hello.git","pushed_at":"2026-01-02T03:04:05Z"}]`))
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	repos, err := github.ListRepos(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if gotAuth != "Bearer ghp_list_test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if len(repos) != 1 {
		t.Fatalf("len = %d", len(repos))
	}
	r := repos[0]
	if r.FullName != "octocat/hello" || !r.Private || r.CloneURL != "https://github.com/octocat/hello.git" {
		t.Fatalf("mapping = %+v", r)
	}
}

func TestListReposRejectedToken(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_revoked"}); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	if _, err := github.ListRepos(context.Background(), nil); !errors.Is(err, github.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}
