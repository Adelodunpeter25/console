// Repo listing for the pickers. The stored PAT that lets git clone also
// authenticates GET /user/repos, so the mobile/desktop GitHub section can
// show the connected account's repositories without a second connection
// flow. Read-only; the token is sent in the Authorization header and never
// appears in an error message.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConnected is returned when no usable token is stored. Callers answer
// 409 so the client points at Accounts rather than implying a bug.
var ErrNotConnected = errors.New("GitHub is not connected. Add a personal access token in Settings > Accounts, then try again")

// Repo is one repository as the pickers show it. CloneURL is the HTTPS clone
// endpoint — what POST /api/projects/clone and /api/sessions/from-repo take
// as their url.
type Repo struct {
	FullName    string `json:"fullName"`
	Name        string `json:"name"`
	Private     bool   `json:"private"`
	Description string `json:"description"`
	HTMLURL     string `json:"htmlUrl"`
	CloneURL    string `json:"cloneUrl"`
	PushedAt    string `json:"pushedAt"`
}

// ListRepos returns the connected account's repositories, most recently
// pushed first. A nil client gets the default (10s timeout) client.
func ListRepos(ctx context.Context, client *http.Client) ([]Repo, error) {
	cred, err := LoadCredential()
	if err != nil || strings.TrimSpace(cred.Token) == "" {
		return nil, ErrNotConnected
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		APIBase+"/user/repos?per_page=100&sort=pushed&direction=desc&affiliation=owner,collaborator,organization_member", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "console-agent")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// The stored token stopped working (revoked, expired). Not a client
		// bug: 409 semantics, reconnect in Accounts.
		return nil, ErrInvalidToken
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("%w GitHub is unavailable right now (%d). Try again shortly.", ErrUnreachable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub returned an unexpected response (%d).", resp.StatusCode)
	}
	if readErr != nil {
		return nil, fmt.Errorf("Could not read the GitHub response: %w", err)
	}

	var parsed []struct {
		FullName    string `json:"full_name"`
		Name        string `json:"name"`
		Private     bool   `json:"private"`
		Description string `json:"description"`
		HTMLURL     string `json:"html_url"`
		CloneURL    string `json:"clone_url"`
		PushedAt    string `json:"pushed_at"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("Could not read the GitHub response: %w", err)
	}
	repos := make([]Repo, 0, len(parsed))
	for _, r := range parsed {
		repos = append(repos, Repo{
			FullName:    r.FullName,
			Name:        r.Name,
			Private:     r.Private,
			Description: r.Description,
			HTMLURL:     r.HTMLURL,
			CloneURL:    r.CloneURL,
			PushedAt:    r.PushedAt,
		})
	}
	return repos, nil
}
