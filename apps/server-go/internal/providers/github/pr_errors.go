// Pull-request read errors. The strings are the user-facing messages
// themselves. ErrUnreachable comes from pat.go, which verifies
// tokens against the same host.
package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrNotConnected means no usable stored token at all.
	ErrNotConnected = fmt.Errorf("GitHub is not connected on this server. Connect it in Settings.")
	// ErrTokenRejected is 401 from GitHub.
	ErrTokenRejected = fmt.Errorf("GitHub rejected the stored token. Reconnect in Settings.")
	// ErrMissingScope is a 403 that is not rate limiting.
	ErrMissingScope = fmt.Errorf("The stored token lacks the scope this needs. Grant it in Settings (repo for classic tokens).")
	// ErrNotFound is 404: no such PR, or no access — GitHub deliberately
	// conflates the two for private repositories.
	ErrNotFound = fmt.Errorf("Pull request not found — or the token has no access to it. GitHub reports both the same way.")
	// ErrUnreachable is pat.go's: GitHub could not be reached at all
	// (DNS, timeout, 5xx), shared so callers can return 502.
)

// RateLimitError marks an exhausted rate limit; ResetAt is when it lifts.
type RateLimitError struct{ ResetAt time.Time }

func (e *RateLimitError) Error() string {
	return "GitHub rate limit reached. Try again after " + e.ResetAt.Local().Format("15:04") + "."
}

// ValidationError carries GitHub's own 422 (or GraphQL validation) message
// verbatim.
type ValidationError struct{ Detail string }

func (e *ValidationError) Error() string {
	if e.Detail == "" {
		return "GitHub rejected the request."
	}
	return e.Detail
}

// mapRESTStatus turns a non-2xx REST status into the shared error vocabulary.
func mapRESTStatus(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrTokenRejected
	case http.StatusForbidden:
		return forbiddenError(resp.Header)
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusUnprocessableEntity:
		return &ValidationError{Detail: restValidationMessage(body)}
	case http.StatusConflict:
		return fmt.Errorf("The branch moved since this pull request was read. Reload and try again.")
	default:
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%w GitHub is unavailable right now (%d). Try again shortly.", ErrUnreachable, resp.StatusCode)
		}
		return fmt.Errorf("GitHub returned an unexpected response (%d).", resp.StatusCode)
	}
}

// forbiddenError separates an exhausted rate limit (reset time known from
// the header) from every other 403, which for a stored token means scope.
func forbiddenError(h http.Header) error {
	if h.Get("X-Ratelimit-Remaining") == "0" {
		reset := time.Now().Add(time.Hour)
		if n, err := parseUnix(h.Get("X-Ratelimit-Reset")); err == nil {
			reset = n
		}
		return &RateLimitError{ResetAt: reset}
	}
	return ErrMissingScope
}

// restValidationMessage digs GitHub's human-readable 422 message out of the
// body so the user sees GitHub's own wording.
func restValidationMessage(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.Message != "" {
		return parsed.Message
	}
	for _, e := range parsed.Errors {
		if e.Message != "" {
			return e.Message
		}
	}
	return ""
}
