// Cloudflare Workers AI transcription client. Audio is held in memory for the
// duration of the request and never written to disk; neither request nor
// response bodies are logged, because both can contain user speech.
package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the Cloudflare API root. Tests point a Client at a local
// fake instead.
const DefaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client talks to Workers AI. It holds no credential: each call resolves the
// current stored/env pair, so changing the config takes effect immediately.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client. An empty baseURL selects Cloudflare; tests pass
// an httptest server URL.
func NewClient(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Transcription is batch work: a near-limit clip can take a while.
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

// Transcribe sends one audio clip to Cloudflare and returns its text. The
// error is safe to show to the user: it never contains the API token.
func (c *Client) Transcribe(ctx context.Context, audio []byte, contentType string) (string, error) {
	if len(audio) == 0 {
		return "", ErrEmptyAudio
	}
	if len(audio) > MaxAudioBytes {
		return "", ErrAudioTooLarge
	}
	creds, err := Resolve()
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf(
		"%s/accounts/%s/ai/run/%s",
		c.baseURL, url.PathEscape(creds.AccountID), Model,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(audio))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+creds.APIToken)
	req.Header.Set("Content-Type", normalizeContentType(contentType))
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read cloudflare response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", upstreamError(resp.StatusCode, body)
	}

	var envelope cloudflareResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", errors.New("cloudflare returned an unreadable response")
	}
	if !envelope.Success {
		return "", fmt.Errorf("cloudflare: %s", errorDetail(envelope.Errors, resp.StatusCode))
	}
	return strings.TrimSpace(envelope.Result.Text), nil
}

type cloudflareResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Text string `json:"text"`
	} `json:"result"`
	Errors []cloudflareErrorDetail `json:"errors"`
}

type cloudflareErrorDetail struct {
	Message string `json:"message"`
}

func normalizeContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "application/octet-stream"
	}
	if strings.HasPrefix(mediaType, "audio/") || mediaType == "application/octet-stream" {
		return mediaType
	}
	return "application/octet-stream"
}

// upstreamError maps the statuses a user can act on to named causes and keeps
// any other Cloudflare detail short enough to surface in a UI.
func upstreamError(status int, body []byte) error {
	var envelope struct {
		Errors []cloudflareErrorDetail `json:"errors"`
	}
	_ = json.Unmarshal(body, &envelope)

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("cloudflare rejected the API token (HTTP %d)", status)
	case http.StatusTooManyRequests:
		return fmt.Errorf("cloudflare rate limit or quota exceeded (HTTP %d)", status)
	case http.StatusRequestEntityTooLarge:
		return fmt.Errorf("cloudflare rejected the audio as too large (HTTP %d)", status)
	}

	detail := errorDetail(envelope.Errors, status)
	if detail == "" {
		return fmt.Errorf("cloudflare request failed (HTTP %d)", status)
	}
	return fmt.Errorf("cloudflare request failed (HTTP %d): %s", status, detail)
}

// errorDetail returns Cloudflare's first error message, capped so a verbose
// upstream body cannot flood the client.
func errorDetail(details []cloudflareErrorDetail, status int) string {
	if len(details) == 0 {
		return fmt.Sprintf("request failed (HTTP %d)", status)
	}
	detail := strings.TrimSpace(details[0].Message)
	if detail == "" {
		return fmt.Sprintf("request failed (HTTP %d)", status)
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return detail
}
