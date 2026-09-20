// webFetch tool: HTTP GET/POST/etc a URL, returning readable text. Port of
// apps/server/agent/src/tools/fetch.ts. GET requests to likely web pages
// try Firecrawl first (clean markdown extraction); everything else, and
// any Firecrawl miss, falls back to a direct HTTP request with naive
// HTML-to-text stripping.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type fetchInput struct {
	URL           string            `json:"url" jsonschema:"required,description=The URL to fetch"`
	Method        string            `json:"method,omitempty" jsonschema:"description=HTTP method (default GET)"`
	Headers       map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP request headers as key-value pairs"`
	Body          string            `json:"body,omitempty" jsonschema:"description=Request body as a string (for POST/PUT/PATCH)"`
	TimeoutMs     int               `json:"timeoutMs,omitempty" jsonschema:"description=Request timeout in milliseconds (default 15000)"`
	MaxBytes      int               `json:"maxBytes,omitempty" jsonschema:"description=Maximum response body size in bytes (default 524288)"`
	ReturnHeaders bool              `json:"returnHeaders,omitempty" jsonschema:"description=Include response headers in the output"`
}

const (
	fetchDefaultTimeoutMs = 15_000
	fetchDefaultMaxBytes  = 512 * 1024
)

var (
	htmlStyleRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlScriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	htmlSpacesRe = regexp.MustCompile(`\s{2,}`)
	htmlBlanksRe = regexp.MustCompile(`\n{3,}`)
)

// htmlToText naively strips HTML tags and collapses whitespace for cleaner
// LLM context. Not a full HTML parser — good enough for plain-text extraction.
func htmlToText(html string) string {
	s := htmlStyleRe.ReplaceAllString(html, "")
	s = htmlScriptRe.ReplaceAllString(s, "")
	s = htmlTagRe.ReplaceAllString(s, " ")
	replacer := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	s = replacer.Replace(s)
	s = htmlSpacesRe.ReplaceAllString(s, " ")
	s = htmlBlanksRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func isLikelyAPIURL(url string, headers map[string]string) bool {
	if strings.Contains(url, "/api/") || strings.HasSuffix(url, ".json") {
		return true
	}
	for _, v := range headers {
		if strings.Contains(v, "application/json") {
			return true
		}
	}
	return false
}

// Fetch performs an HTTP request and returns readable text. GET requests to
// likely web pages try Firecrawl first; everything else (and any Firecrawl
// miss) uses a direct request with content-type-aware formatting.
var Fetch = NewTool("webFetch", "Fetch content from a URL or web page as markdown or text.", TierRead,
	func(ctx context.Context, in fetchInput) (any, error) {
		if in.URL == "" {
			return nil, NewToolError("url is required")
		}
		method := in.Method
		if method == "" {
			method = http.MethodGet
		}
		timeoutMs := in.TimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = fetchDefaultTimeoutMs
		}
		maxBytes := in.MaxBytes
		if maxBytes <= 0 {
			maxBytes = fetchDefaultMaxBytes
		}

		reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()

		isGet := method == http.MethodGet && in.Body == ""
		if isGet && !isLikelyAPIURL(in.URL, in.Headers) {
			markdown, title, sourceURL, statusCode, ok, err := firecrawlScrape(reqCtx, in.URL)
			if err == nil && ok {
				sections := []string{
					fmt.Sprintf("URL: %s", in.URL),
					fmt.Sprintf("Status: %d OK (via Firecrawl)", statusCode),
					fmt.Sprintf("Title: %s", title),
				}
				if in.ReturnHeaders {
					sections = append(sections, fmt.Sprintf("SourceURL: %s", sourceURL))
				}
				sections = append(sections, "", "Body (markdown):", markdown)
				return strings.Join(sections, "\n"), nil
			}
			// Firecrawl error or empty result — fall through to direct fetch.
		}

		var bodyReader io.Reader
		if in.Body != "" {
			bodyReader = strings.NewReader(in.Body)
		}
		req, err := http.NewRequestWithContext(reqCtx, method, in.URL, bodyReader)
		if err != nil {
			return nil, NewToolError("Invalid request: %v", err)
		}
		for k, v := range in.Headers {
			req.Header.Set(k, v)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if reqCtx.Err() == context.DeadlineExceeded {
				return fmt.Sprintf("Error: Request timed out after %dms — %s", timeoutMs, in.URL), nil
			}
			return fmt.Sprintf("Error: %v", err), nil
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
		if err != nil {
			return nil, NewToolError("Error reading response body: %v", err)
		}
		truncated := len(data) > maxBytes
		if truncated {
			data = data[:maxBytes]
		}
		bodyText := string(data)
		if truncated {
			bodyText += fmt.Sprintf("\n\n[... response truncated: showing first %d bytes ...]", maxBytes)
		}

		var formattedBody string
		switch {
		case strings.Contains(contentType, "application/json") || strings.Contains(contentType, "+json"):
			var parsed any
			if err := json.Unmarshal([]byte(bodyText), &parsed); err == nil {
				pretty, _ := json.MarshalIndent(parsed, "", "  ")
				formattedBody = string(pretty)
			} else {
				formattedBody = bodyText
			}
		case strings.Contains(contentType, "text/html"):
			formattedBody = htmlToText(bodyText)
		default:
			formattedBody = bodyText
		}

		sections := []string{
			fmt.Sprintf("URL: %s", in.URL),
			fmt.Sprintf("Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
			fmt.Sprintf("Content-Type: %s", contentType),
		}
		if in.ReturnHeaders {
			var headerLines []string
			for k, values := range resp.Header {
				headerLines = append(headerLines, fmt.Sprintf("  %s: %s", k, strings.Join(values, ", ")))
			}
			if len(headerLines) > 0 {
				sections = append(sections, "Headers:\n"+strings.Join(headerLines, "\n"))
			}
		}
		sections = append(sections, "", "Body:", formattedBody)

		result := strings.Join(sections, "\n")
		if resp.StatusCode >= 400 {
			return nil, NewToolError("%s", result)
		}
		return result, nil
	})
