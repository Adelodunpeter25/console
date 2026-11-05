// Shared HTTP transport for pull-request reads: one GraphQL request
// helper, one REST request helper, one client with a timeout and pinned
// headers. The token travels per call and is never logged, returned, or
// embedded in an error string.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GraphQLBase is the GitHub GraphQL root. A var, not a const, so tests can
// point it at an httptest server; production code never assigns it.
var GraphQLBase = "https://api.github.com/graphql"

// pullRequestClient is the shared HTTP client for PR reads.
var pullRequestClient = &http.Client{Timeout: 15 * time.Second}

// graphqlRequest posts one GraphQL document and decodes the `data` field.
// GraphQL failures come back as HTTP 200 with an errors array (or a plain
// 403 when rate limited), so both paths map through the same error
// vocabulary.
func graphqlRequest(ctx context.Context, token, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GraphQLBase, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	setGitHubHeaders(req, token)

	resp, err := pullRequestClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrTokenRejected
	case resp.StatusCode == http.StatusForbidden:
		return forbiddenError(resp.Header)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w GitHub is unavailable right now (%d). Try again shortly.", ErrUnreachable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("GitHub returned an unexpected response (%d).", resp.StatusCode)
	}
	if readErr != nil {
		return fmt.Errorf("Could not read the GitHub response: %w", readErr)
	}

	var parsed struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("Could not read the GitHub response: %w", err)
	}
	for _, e := range parsed.Errors {
		switch e.Type {
		case "RATE_LIMITED":
			return &RateLimitError{ResetAt: time.Now().Add(time.Minute)} // GraphQL does not report a reset time
		case "NOT_FOUND":
			return ErrNotFound
		case "INSUFFICIENT_SCOPES", "FORBIDDEN":
			return ErrMissingScope
		}
		// A query-level validation error; surface the message.
		return &ValidationError{Detail: e.Message}
	}
	if len(parsed.Errors) > 0 || len(parsed.Data) == 0 {
		return fmt.Errorf("GitHub returned no data for this query.")
	}
	return json.Unmarshal(parsed.Data, out)
}

// restGet performs one GET against the REST API and decodes the body.
func restGet(ctx context.Context, token, path string, query url.Values, out any) error {
	u := APIBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	setGitHubHeaders(req, token)

	resp, err := pullRequestClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))

	if err := mapRESTStatus(resp, body); err != nil {
		return err
	}
	if readErr != nil {
		return fmt.Errorf("Could not read the GitHub response: %w", readErr)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("Could not read the GitHub response: %w", err)
	}
	return nil
}

// setGitHubHeaders applies the shared request headers: bearer token, the
// GitHub JSON accept type, and the pinned API version.
func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "console-agent")
	req.Header.Set("Content-Type", "application/json")
}

func parseUnix(s string) (time.Time, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0), nil
}
