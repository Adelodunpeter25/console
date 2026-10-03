// Personal access token validation. A pasted token is checked against
// GET api.github.com/user before it is stored, so the UI never shows a
// connection that cannot actually authenticate git. The login and scopes are
// cached in the credential file at this point (see credentials.go) which keeps
// later status reads offline.
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

// APIBase is the GitHub REST root. A var, not a const, so tests can point it
// at an httptest server; production code never assigns it.
var APIBase = "https://api.github.com"

// errNoToken is returned for an empty/blank token. Callers map it to 400.
var errNoToken = errors.New("GitHub token is empty.")

// ErrInvalidToken marks a token GitHub actively rejected (401). Callers map it
// to 400 so the UI can say "that token is wrong" rather than "try again".
var ErrInvalidToken = errors.New("GitHub rejected that token.")

// ErrUnreachable marks a failure to reach GitHub at all (DNS, timeout, 5xx).
// Separate from ErrInvalidToken so callers can return 502: telling the user
// to paste a different token would not help, and a 400 would invite a retry
// loop against a working credential.
var ErrUnreachable = errors.New("Could not reach GitHub to verify the token.")

// ValidatePAT checks a pasted token against api.github.com/user and returns
// the account login plus the scopes GitHub reports.
//
// Scopes are advisory: classic PATs and OAuth tokens return X-OAuth-Scopes,
// but fine-grained PATs return no such header at all. An empty scope list is
// therefore normal and never an error — it is not a usable authorization
// signal either way, so callers must not gate on it.
func ValidatePAT(ctx context.Context, client *http.Client, token string) (login string, scopes []string, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil, errNoToken
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/user", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "console-agent")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	// Read once. The body can echo request details on some failures, so it is
	// used only for the 200-path parse below and never echoed to the client.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return "", nil, ErrInvalidToken
	case resp.StatusCode == http.StatusForbidden:
		// Rate limiting or a blocked token: not the caller's input, and not
		// proof the token is wrong.
		return "", nil, fmt.Errorf("%w GitHub refused the verification request (rate limited or token blocked). Try again shortly.", ErrUnreachable)
	case resp.StatusCode >= 500:
		return "", nil, fmt.Errorf("%w GitHub is unavailable right now (%d). Try again shortly.", ErrUnreachable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", nil, fmt.Errorf("GitHub returned an unexpected response (%d).", resp.StatusCode)
	}
	if readErr != nil {
		return "", nil, fmt.Errorf("Could not read the GitHub response: %w", readErr)
	}

	var parsed struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", nil, fmt.Errorf("Could not read the GitHub response: %w", err)
	}
	if strings.TrimSpace(parsed.Login) == "" {
		return "", nil, fmt.Errorf("GitHub did not return an account for that token.")
	}
	return strings.TrimSpace(parsed.Login), ParseScopes(resp.Header.Get("X-OAuth-Scopes")), nil
}

// ParseScopes splits GitHub's comma-separated X-OAuth-Scopes header. Absent or
// blank (fine-grained PATs) yields nil, never an error.
func ParseScopes(header string) []string {
	if strings.TrimSpace(header) == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
