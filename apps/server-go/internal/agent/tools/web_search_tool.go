// webSearch tool: keyless Firecrawl search first, DuckDuckGo HTML scrape on
// retryable failure, or Brave (requires BRAVE_SEARCH_API_KEY) when
// explicitly requested. Port of apps/server/agent/src/tools/web-search.ts.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type webSearchInput struct {
	Query        string `json:"query" jsonschema:"required,description=The search query"`
	NumResults   int    `json:"numResults,omitempty" jsonschema:"description=Number of results to return (1-20, default 5)"`
	SearchEngine string `json:"searchEngine,omitempty" jsonschema:"description=\"duckduckgo\" (default) or \"brave\""`
}

type webSearchResult struct {
	Title       string
	URL         string
	Snippet     string
	Markdown    string
	Description string
}

var (
	ddgResultBlockRe = regexp.MustCompile(`(?s)<div class="result[^"]*"[^>]*>(.*?)</div>\s*</div>`)
	ddgTitleURLRe    = regexp.MustCompile(`(?s)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippetRe     = regexp.MustCompile(`(?s)<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
	htmlEntityRe     = regexp.MustCompile(`&[a-zA-Z#0-9]+;`)
)

var htmlEntities = map[string]string{
	"&amp;": "&", "&lt;": "<", "&gt;": ">", "&#x27;": "'", "&nbsp;": " ",
}

func decodeHTMLEntities(s string) string {
	return htmlEntityRe.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := htmlEntities[m]; ok {
			return v
		}
		return m
	})
}

// duckDuckGoURL is a var (not const) so tests can point it at a local
// server instead of the real DuckDuckGo endpoint.
var duckDuckGoURL = "https://html.duckduckgo.com/html/"

// SetDuckDuckGoURLForTest overrides the DuckDuckGo lite endpoint; returns a
// restore function. Test-only hook, not used by production code paths.
func SetDuckDuckGoURLForTest(url string) (restore func()) {
	prev := duckDuckGoURL
	duckDuckGoURL = url
	return func() { duckDuckGoURL = prev }
}

var braveSearchURL = "https://api.search.brave.com/res/v1/web/search"

// SetBraveSearchURLForTest overrides the Brave Search endpoint; returns a
// restore function. Test-only hook, not used by production code paths.
func SetBraveSearchURLForTest(url string) (restore func()) {
	prev := braveSearchURL
	braveSearchURL = url
	return func() { braveSearchURL = prev }
}

// searchDuckDuckGo scrapes the no-JS "lite" HTML results page — no API key
// required, best-effort regex parsing (mirrors the TS scraper).
func searchDuckDuckGo(ctx context.Context, query string, numResults int) ([]webSearchResult, error) {
	reqURL := duckDuckGoURL + "?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AgentHarness/1.0; +https://github.com/console-agent)")
	req.Header.Set("Accept", "text/html")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	html := string(body)

	var results []webSearchResult
	for _, block := range ddgResultBlockRe.FindAllStringSubmatch(html, -1) {
		if len(results) >= numResults {
			break
		}
		titleMatch := ddgTitleURLRe.FindStringSubmatch(block[1])
		if titleMatch == nil {
			continue
		}
		rawURL := titleMatch[1]
		title := decodeHTMLEntities(htmlTagRe.ReplaceAllString(titleMatch[2], ""))
		title = strings.TrimSpace(title)
		snippet := ""
		if snippetMatch := ddgSnippetRe.FindStringSubmatch(block[1]); snippetMatch != nil {
			snippet = strings.TrimSpace(decodeHTMLEntities(htmlTagRe.ReplaceAllString(snippetMatch[1], "")))
		}

		finalURL := rawURL
		if parsed, err := url.Parse(rawURL); err == nil {
			if uddg := parsed.Query().Get("uddg"); uddg != "" {
				if decoded, err := url.QueryUnescape(uddg); err == nil {
					finalURL = decoded
				}
			}
		}
		if title != "" && finalURL != "" {
			results = append(results, webSearchResult{Title: title, URL: finalURL, Snippet: snippet})
		}
	}
	return results, nil
}

type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}
type braveResponse struct {
	Web struct {
		Results []braveWebResult `json:"results"`
	} `json:"web"`
}

// searchBrave calls Brave's Web Search API; requires BRAVE_SEARCH_API_KEY.
func searchBrave(ctx context.Context, query string, numResults int) ([]webSearchResult, error) {
	apiKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("Brave Search requires a BRAVE_SEARCH_API_KEY environment variable. Get a free key at https://api.search.brave.com/register")
	}
	reqURL := fmt.Sprintf("%s?q=%s&count=%d", braveSearchURL, url.QueryEscape(query), numResults)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Brave Search returned %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var parsed braveResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	results := parsed.Web.Results
	if len(results) > numResults {
		results = results[:numResults]
	}
	out := make([]webSearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, webSearchResult{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	return out, nil
}

func renderFirecrawlResults(query string, results []FirecrawlSearchResult) string {
	lines := []string{fmt.Sprintf("Web search results for: %q (%d results) — via Firecrawl (markdown)\n", query, len(results))}
	for i, r := range results {
		lines = append(lines, fmt.Sprintf("[%d] %s", i+1, r.Title))
		lines = append(lines, fmt.Sprintf("    URL: %s", r.URL))
		if r.Description != "" {
			lines = append(lines, "    "+r.Description)
		}
		if r.Markdown != "" {
			lines = append(lines, "    ---", r.Markdown)
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func renderSnippetResults(query string, results []webSearchResult) string {
	lines := []string{fmt.Sprintf("Web search results for: %q (%d results)\n", query, len(results))}
	for i, r := range results {
		lines = append(lines, fmt.Sprintf("[%d] %s", i+1, r.Title))
		lines = append(lines, fmt.Sprintf("    URL: %s", r.URL))
		if r.Snippet != "" {
			lines = append(lines, "    "+r.Snippet)
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

const webSearchTimeout = 15 * time.Second

// WebSearch searches the web for up-to-date documentation and information.
// Default engine tries keyless Firecrawl first (full markdown per result),
// falling back to DuckDuckGo's HTML lite page on a retryable Firecrawl
// error or empty result set; "brave" bypasses Firecrawl entirely.
var WebSearch = NewTool("webSearch", "Search the web for up-to-date documentation and information.", TierRead,
	func(ctx context.Context, in webSearchInput) (any, error) {
		if strings.TrimSpace(in.Query) == "" {
			return nil, NewToolError("query is required")
		}
		numResults := in.NumResults
		if numResults <= 0 {
			numResults = 5
		}
		if numResults > 20 {
			numResults = 20
		}

		reqCtx, cancel := context.WithTimeout(ctx, webSearchTimeout)
		defer cancel()

		if in.SearchEngine == "brave" {
			results, err := searchBrave(reqCtx, in.Query, numResults)
			if err != nil {
				return nil, NewToolError("%v", err)
			}
			if len(results) == 0 {
				return fmt.Sprintf("No results found for: %q", in.Query), nil
			}
			return renderSnippetResults(in.Query, results), nil
		}

		fcResults, ok, fcErr := firecrawlSearch(reqCtx, in.Query, numResults)
		if fcErr == nil && ok {
			return renderFirecrawlResults(in.Query, fcResults), nil
		}
		if fcErr != nil && !isRetryableFirecrawlError(fcErr) {
			return nil, NewToolError("%v", fcErr)
		}
		// Firecrawl errored retryably, or succeeded with zero results — fall
		// back to DuckDuckGo.
		results, err := searchDuckDuckGo(reqCtx, in.Query, numResults)
		if err != nil {
			return nil, NewToolError("%v", err)
		}
		if len(results) == 0 {
			return fmt.Sprintf("No results found for: %q", in.Query), nil
		}
		return renderSnippetResults(in.Query, results), nil
	})
