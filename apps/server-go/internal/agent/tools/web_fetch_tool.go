// webFetch tool: HTTP GET a URL, returning readable text.
// Direct HTTP fetch with HTML-to-text / JSON formatting is the default.
// When FIRECRAWL_API_KEY or FIRECRAWL_BASE_URL is configured, GET requests
// to web pages try Firecrawl first for markdown extraction and fall back to
// direct fetch on error.
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
	URL       string `json:"url" jsonschema:"required,description=The URL to fetch"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"description=Request timeout in milliseconds (default 15000)"`
	MaxBytes  int    `json:"maxBytes,omitempty" jsonschema:"description=Maximum response body size in bytes (default 524288)"`
}

const (
	fetchDefaultTimeoutMs = 15_000
	fetchDefaultMaxBytes  = 512 * 1024
	// fetchSpillMaxBytes caps the read for the spill file. Beyond this the
	// rest of the body is genuinely discarded: 5 MB of one page is already
	// far past what any follow-up read would want.
	fetchSpillMaxBytes = 5 << 20
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

func isLikelyAPIURL(url string) bool {
	return strings.Contains(url, "/api/") || strings.HasSuffix(url, ".json")
}

// Fetch performs an HTTP request and returns readable text.
var Fetch = NewTool("webFetch", "Fetch content from a URL or web page as markdown or text.", TierRead,
	func(ctx context.Context, in fetchInput) (any, error) {
		if in.URL == "" {
			return nil, NewToolError("url is required")
		}
		method := http.MethodGet
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

		if IsFirecrawlConfigured() && !isLikelyAPIURL(in.URL) {
			markdown, title, _, statusCode, ok, err := firecrawlScrape(reqCtx, in.URL)
			if err == nil && ok {
				sections := []string{
					fmt.Sprintf("URL: %s", in.URL),
					fmt.Sprintf("Status: %d OK (via Firecrawl)", statusCode),
					fmt.Sprintf("Title: %s", title),
				}
				sections = append(sections, "", "Body (markdown):", markdown)
				return textResult(strings.Join(sections, "\n")), nil
			}
			// Firecrawl error or empty result — fall through to direct fetch.
		}

		req, err := http.NewRequestWithContext(reqCtx, method, in.URL, nil)
		if err != nil {
			return nil, NewToolError("Invalid request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if reqCtx.Err() == context.DeadlineExceeded {
				return textResult(fmt.Sprintf("Error: Request timed out after %dms — %s", timeoutMs, in.URL)), nil
			}
			return textResult(fmt.Sprintf("Error: %v", err)), nil
		}
		defer resp.Body.Close()

		contentType := resp.Header.Get("Content-Type")
		data, err := io.ReadAll(io.LimitReader(resp.Body, fetchSpillMaxBytes+1))
		if err != nil {
			return nil, NewToolError("Error reading response body: %v", err)
		}
		hardTruncated := len(data) > fetchSpillMaxBytes
		if hardTruncated {
			data = data[:fetchSpillMaxBytes]
		}
		bodyText := string(data)
		// Over the display cap: keep the head for the model (the start of a
		// page is the useful side), spill the full body to a file it can
		// re-read or grep.
		if len(data) > maxBytes {
			if path, spillErr := spillToFile(spillDir(""), spillURLName(in.URL), ".body.log", bodyText); spillErr == nil {
				bodyText = bodyText[:maxBytes] + fmt.Sprintf(
					"\n\n[... response truncated: showing first %s of %s. Full body saved to %s ...]",
					humanBytes(maxBytes), humanBytes(len(data)), path)
			} else {
				bodyText += fmt.Sprintf("\n\n[... response truncated: showing first %d bytes ...]", maxBytes)
			}
		} else if hardTruncated {
			bodyText += fmt.Sprintf("\n\n[... response truncated at %s ...]", humanBytes(fetchSpillMaxBytes))
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
		sections = append(sections, "", "Body:", formattedBody)

		result := strings.Join(sections, "\n")
		return Envelope{Content: textResult(result), IsError: resp.StatusCode >= 400}, nil
	})
