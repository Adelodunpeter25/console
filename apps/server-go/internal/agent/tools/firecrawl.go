// Firecrawl HTTP client — opt-in. Uses
// https://api.firecrawl.dev/v1 when FIRECRAWL_API_KEY is set, or a custom
// self-hosted endpoint when FIRECRAWL_BASE_URL is set. Without either,
// callers skip Firecrawl and use custom built-in search/fetch implementations.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	defaultFirecrawlBaseURL = "https://api.firecrawl.dev/v1"
	firecrawlMaxMarkdown    = 25_000
	firecrawlTruncateNotice = "\n\n[...content truncated...]"
)

// IsFirecrawlConfigured returns true if an API key or custom endpoint is configured.
func IsFirecrawlConfigured() bool {
	return os.Getenv("FIRECRAWL_API_KEY") != "" || os.Getenv("FIRECRAWL_BASE_URL") != ""
}

func truncateMarkdown(text string) string {
	if len(text) <= firecrawlMaxMarkdown {
		return text
	}
	return text[:firecrawlMaxMarkdown] + firecrawlTruncateNotice
}

func firecrawlBaseURL() string {
	if base := strings.TrimRight(os.Getenv("FIRECRAWL_BASE_URL"), "/"); base != "" {
		return base
	}
	return defaultFirecrawlBaseURL
}

func firecrawlHeaders() map[string]string {
	headers := map[string]string{"Content-Type": "application/json"}
	if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	return headers
}

func firecrawlPost(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, firecrawlBaseURL()+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	for k, v := range firecrawlHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		snippet := respBody
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		return nil, fmt.Errorf("firecrawl %s error: HTTP %d %s", path, resp.StatusCode, snippet)
	}
	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// firecrawlScrape fetches url as clean markdown via Firecrawl's /scrape
// endpoint. Returns ok=false (not an error) when Firecrawl doesn't have
// markdown for the page, so the caller can fall back to a direct fetch.
func firecrawlScrape(ctx context.Context, url string) (markdown, title, sourceURL string, statusCode int, ok bool, err error) {
	json, err := firecrawlPost(ctx, "/scrape", map[string]any{
		"url": url, "formats": []string{"markdown"}, "onlyMainContent": true,
	})
	if err != nil {
		return "", "", "", 0, false, err
	}
	success, _ := json["success"].(bool)
	data, _ := json["data"].(map[string]any)
	if !success || data == nil {
		return "", "", "", 0, false, nil
	}
	md, _ := data["markdown"].(string)
	if md == "" {
		return "", "", "", 0, false, nil
	}
	meta, _ := data["metadata"].(map[string]any)
	if t, ok := meta["title"].(string); ok {
		title = t
	}
	if s, ok := meta["sourceURL"].(string); ok {
		sourceURL = s
	}
	statusCode = 200
	if sc, ok := meta["statusCode"].(float64); ok {
		statusCode = int(sc)
	}
	return truncateMarkdown(md), title, sourceURL, statusCode, true, nil
}

// FirecrawlSearchResult is one hit from firecrawlSearch.
type FirecrawlSearchResult struct {
	Title       string
	URL         string
	Markdown    string
	Description string
}

// firecrawlSearch runs a web search via Firecrawl's /search endpoint,
// returning full markdown per result (richer than snippet-only engines).
// ok=false (not an error) means Firecrawl succeeded but found nothing.
func firecrawlSearch(ctx context.Context, query string, limit int) ([]FirecrawlSearchResult, bool, error) {
	json, err := firecrawlPost(ctx, "/search", map[string]any{
		"query": query, "limit": limit,
		"scrapeOptions": map[string]any{"formats": []string{"markdown"}, "onlyMainContent": true},
	})
	if err != nil {
		return nil, false, err
	}
	success, _ := json["success"].(bool)
	data, _ := json["data"].([]any)
	if !success || len(data) == 0 {
		return nil, false, nil
	}
	out := make([]FirecrawlSearchResult, 0, len(data))
	for _, item := range data {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		url, _ := m["url"].(string)
		if url == "" {
			continue
		}
		title, _ := m["title"].(string)
		description, _ := m["description"].(string)
		markdown, _ := m["markdown"].(string)
		out = append(out, FirecrawlSearchResult{
			Title: title, URL: url, Markdown: truncateMarkdown(markdown), Description: description,
		})
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

// isRetryableFirecrawlError reports whether a Firecrawl error (rate limit,
// auth/access/suspicious IP block, network failure, 5xx) should trigger
// fallback to built-in handlers rather than hard-failing the tool call.
func isRetryableFirecrawlError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") {
		return true
	}
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") {
		return true
	}
	if strings.Contains(msg, "suspicious") || strings.Contains(msg, "captcha") || strings.Contains(msg, "blocked") {
		return true
	}
	if strings.Contains(msg, "http 5") || strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return true
	}
	if strings.Contains(msg, "network") || strings.Contains(msg, "connection") || strings.Contains(msg, "timeout") {
		return true
	}
	return false
}
