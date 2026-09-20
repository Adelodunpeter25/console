// Firecrawl HTTP client — keyless-first. Port of
// apps/server/agent/src/tools/firecrawl.ts. Uses
// https://api.firecrawl.dev/v1 without Authorization by default; sends a
// Bearer token when FIRECRAWL_API_KEY is set. Supports self-hosted via
// FIRECRAWL_BASE_URL.
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
