package opentunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultAPI is the hosted OpenTunnel service.
const DefaultAPI = "https://opentunnel.xyz"

const (
	userAgent    = "console-opentunnel/1"
	maxRetries   = 6
	retryBase    = 250 * time.Millisecond
	retryCeiling = 8 * time.Second
)

// APIError is an error answer from the tunnel API.
type APIError struct {
	Status     int
	Tag        string
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("opentunnel: HTTP %d", e.Status)
	}
	return "opentunnel: " + e.Message
}

// IsGone reports whether the tunnel (or its token) no longer exists, which is
// the one failure that costs the public URL.
func IsGone(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusUnauthorized)
}

// cloudflareMessages explains the 52x answers Cloudflare gives before a
// request reaches the tunnel server.
var cloudflareMessages = map[int]string{
	520: "Cloudflare 520: the server returned an unknown error",
	521: "Cloudflare 521: the server refused the connection",
	522: "Cloudflare 522: the connection to the server timed out",
	523: "Cloudflare 523: the server is unreachable",
	524: "Cloudflare 524: the server did not answer in time",
	525: "Cloudflare 525: the SSL handshake with the server failed",
	526: "Cloudflare 526: the server's certificate is invalid",
	527: "Cloudflare 527: the connection to the server failed",
}

type tunnelInfo struct {
	ID            string `json:"id"`
	Hostname      string `json:"hostname"`
	State         string `json:"state"`
	CertificateID string `json:"certificateID,omitempty"`
}

type createResponse struct {
	Tunnel tunnelInfo `json:"tunnel"`
	Token  string     `json:"token"`
}

type certificateState struct {
	Type        string `json:"type"`
	Certificate string `json:"certificate,omitempty"`
	Chain       string `json:"chain,omitempty"`
	Expiry      string `json:"expiry,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type certificateInfo struct {
	ID    string           `json:"id"`
	State certificateState `json:"state"`
}

// api is the HTTP side of the protocol (docs/protocol.md in the OpenTunnel
// repository): create a tunnel, bind a CSR, read the certificate.
type api struct {
	base *url.URL
	http *http.Client
}

func newAPI(base string, client *http.Client) (*api, error) {
	if strings.TrimSpace(base) == "" {
		base = DefaultAPI
	}
	parsed, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("opentunnel: invalid API address %q", base)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &api{base: parsed, http: client}, nil
}

func (a *api) url(path string) string {
	u := *a.base
	u.Path = strings.TrimRight(a.base.Path, "/") + path
	return u.String()
}

// create makes a new tunnel. Repeating a create that may have arrived would
// leak a tunnel, so it is retried only when it cannot have been delivered.
func (a *api) create(ctx context.Context) (createResponse, error) {
	var out createResponse
	err := a.do(ctx, http.MethodPost, "/api/tunnel", "", struct{}{}, false, &out, http.StatusCreated)
	return out, err
}

// bindCertificate submits the CSR. Binding the CSR a tunnel already has just
// answers with its current certificate, so this is safe to repeat.
func (a *api) bindCertificate(ctx context.Context, id, token, csr string) (certificateInfo, error) {
	var out certificateInfo
	err := a.do(ctx, http.MethodPost, "/api/tunnel/"+url.PathEscape(id)+"/certificate", token,
		map[string]string{"csr": csr}, true, &out, http.StatusAccepted, http.StatusOK)
	return out, err
}

func (a *api) certificate(ctx context.Context, id, token string) (certificateInfo, error) {
	var out certificateInfo
	err := a.do(ctx, http.MethodGet, "/api/tunnel/"+url.PathEscape(id)+"/certificate", token, nil, true, &out, http.StatusOK)
	return out, err
}

func (a *api) tunnel(ctx context.Context, id, token string) (tunnelInfo, error) {
	var out tunnelInfo
	err := a.do(ctx, http.MethodGet, "/api/tunnel/"+url.PathEscape(id), token, nil, true, &out, http.StatusOK)
	return out, err
}

// do sends one request, retrying transient failures with jittered
// exponential backoff. idempotent requests may be repeated after any network
// error or 5xx; the others only when the request cannot have arrived.
func (a *api) do(ctx context.Context, method, path, token string, body any, idempotent bool, out any, ok ...int) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, a.url(path), reader)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := a.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < maxRetries && (idempotent || isDialError(err)) {
				if err := sleepCtx(ctx, retryDelay(attempt, 0)); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("opentunnel: %s %s: %w", method, path, err)
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()

		for _, status := range ok {
			if resp.StatusCode == status {
				if out == nil || len(data) == 0 {
					return nil
				}
				if err := json.Unmarshal(data, out); err != nil {
					return fmt.Errorf("opentunnel: decode %s answer: %w", path, err)
				}
				return nil
			}
		}

		apiErr := parseAPIError(resp, data)
		if attempt < maxRetries && retryableStatus(idempotent, resp.StatusCode) {
			if err := sleepCtx(ctx, retryDelay(attempt, apiErr.RetryAfter)); err != nil {
				return err
			}
			continue
		}
		return apiErr
	}
}

// retryableStatus follows the protocol's retry table: idempotent requests
// retry on 429 and any 5xx, the rest only where the request cannot have
// arrived (429, 503, and Cloudflare's 521-523 and 525).
func retryableStatus(idempotent bool, status int) bool {
	if idempotent {
		return status == http.StatusTooManyRequests || status >= 500
	}
	switch status {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, 521, 522, 523, 525:
		return true
	}
	return false
}

func isDialError(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

// retryDelay is a random wait between half of and all of
// min(250ms * 2^attempt, 8s), or Retry-After (at most 8s) when given.
func retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > retryCeiling {
			return retryCeiling
		}
		return retryAfter
	}
	if attempt > 16 {
		attempt = 16
	}
	base := retryBase << attempt
	if base > retryCeiling {
		base = retryCeiling
	}
	return base/2 + time.Duration(rand.Int63n(int64(base/2)+1))
}

func parseAPIError(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{Status: resp.StatusCode}
	if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
		apiErr.RetryAfter = time.Duration(seconds) * time.Second
	}
	var parsed struct {
		Tag     string `json:"_tag"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Message != "" {
		apiErr.Tag, apiErr.Message = parsed.Tag, parsed.Message
		return apiErr
	}
	if msg, ok := cloudflareMessages[resp.StatusCode]; ok {
		apiErr.Message = msg
		return apiErr
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		apiErr.Message = text
	} else {
		apiErr.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return apiErr
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
