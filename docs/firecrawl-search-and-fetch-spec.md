# Firecrawl Web Search & Fetch Integration Specification

**Status**: Draft  
**Applies to**: Server (`apps/server/agent/src/tools/`)  
**Target Capabilities**: Keyless Web Search, JavaScript-Rendered Scraping, Clean Markdown Extraction, Zero-Config Operation

---

## 1. Overview & Keyless Philosophy

The agent requires fast, reliable web searching and page fetching without requiring users to sign up for accounts, pay subscriptions, or configure API keys.

### 1.1 Zero-Configuration (No API Key Required)
Firecrawl provides a **Keyless Tier** on `https://api.firecrawl.dev/v1/search` and `https://api.firecrawl.dev/v1/scrape` that operates **without an API key** (omitting the `Authorization` header). Additionally, developers running self-hosted Firecrawl instances via Docker connect with no authentication.

**Design Rule**: Firecrawl integration in `@console/server` is **100% keyless by default**. No setup or API keys are required for search or fetch to work.

### 1.2 Limitations of Previous Basic Tools
- **`web-search`**: Scraped DuckDuckGo Lite HTML via regex; returned only short 1–2 sentence text snippets, easily rate-limited or blocked.
- **`fetch`**: Made raw `fetch()` calls and naively stripped HTML tags; failed completely on client-side SPAs (Next.js, Mintlify, React docs) and lost markdown formatting.

### 1.3 How Keyless Firecrawl Solves This
1. **Single-Step Search + Markdown Extraction**: `/v1/search` finds top results and scrapes full, clean Markdown for each page in one single request.
2. **Headless Browser Execution**: Executes JavaScript on dynamic sites (SPAs, documentation portals) and strips ads, navigation bars, and cookie prompts.
3. **Clean LLM Markdown**: Produces GitHub-flavored Markdown (preserving tables, code blocks, and headings).

---

## 2. API Endpoints & Request Formats (Keyless)

### 2.1 Web Search (`POST /v1/search`)

```http
POST https://api.firecrawl.dev/v1/search
Content-Type: application/json

{
  "query": "gpui rust text rendering guide",
  "limit": 3,
  "scrapeOptions": {
    "formats": ["markdown"],
    "onlyMainContent": true
  }
}
```

> [!NOTE]
> Notice the absence of the `Authorization` header. The request is processed by Firecrawl's keyless tier directly.

#### Response Structure:
```json
{
  "success": true,
  "data": [
    {
      "title": "Text Layout and Rendering - Zed / GPUI",
      "url": "https://zed.dev/docs/gpui/text",
      "markdown": "# Text Layout and Rendering\n\nGPUI uses Cosmic Text for shaping and rendering...\n\n```rust\nlet run = text_system.layout(...);\n```",
      "description": "Learn how GPUI handles text layout, shaping, and font fallback."
    }
  ]
}
```

---

### 2.2 Page Scraper (`POST /v1/scrape`)

```http
POST https://api.firecrawl.dev/v1/scrape
Content-Type: application/json

{
  "url": "https://docs.rs/tokio/latest/tokio/",
  "formats": ["markdown"],
  "onlyMainContent": true
}
```

#### Response Structure:
```json
{
  "success": true,
  "data": {
    "markdown": "# Crate tokio\n\nTokio is an asynchronous runtime for the Rust programming language...",
    "metadata": {
      "title": "tokio - Rust",
      "sourceURL": "https://docs.rs/tokio/latest/tokio/",
      "statusCode": 200
    }
  }
}
```

---

## 3. Tool Architecture & Fallback Chain

```mermaid
flowchart TD
    A[Agent Calls: web-search] --> B[Firecrawl /v1/search (Keyless)]
    B -- Success --> C[Return Search Results + Full Page Markdown]
    B -- 429 Rate Limit / Network Failure --> D[Fallback: DuckDuckGo Lite Scraper]
    D --> E[Return Search Snippets]
```

```mermaid
flowchart TD
    A[Agent Calls: fetch] --> B{Is HTML web page & GET?}
    B -- Yes --> C[Firecrawl /v1/scrape (Keyless)]
    C -- Success --> D[Return Clean JS-Rendered Markdown]
    C -- Error / Rate Limit --> E[Direct HTTP Fetch + Local HTML Tag Stripper]
    
    B -- No (REST API / POST / Raw JSON) --> E
    E --> F[Return Response Text / Body]
```

---

## 4. Implementation Details (`apps/server/agent/src/tools/`)

### 4.1 Firecrawl Helper Module (`firecrawl.ts`)
```typescript
const FIRECRAWL_BASE_URL =
  process.env.FIRECRAWL_BASE_URL || "https://api.firecrawl.dev/v1";
const FIRECRAWL_API_KEY = process.env.FIRECRAWL_API_KEY; // Optional, only if user explicitly configures one

export async function firecrawlSearch(query: string, limit = 5, signal?: AbortSignal) {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (FIRECRAWL_API_KEY) {
    headers["Authorization"] = `Bearer ${FIRECRAWL_API_KEY}`;
  }

  const response = await fetch(`${FIRECRAWL_BASE_URL}/search`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      query,
      limit,
      scrapeOptions: { formats: ["markdown"], onlyMainContent: true },
    }),
    signal,
  });

  if (!response.ok) {
    throw new Error(`Firecrawl search error: HTTP ${response.status}`);
  }

  return response.json();
}
```

### 4.2 Safe Token Budget & Output Limits
- **Max Content Truncation**: When scraping markdown via search, each page is truncated to `25,000` characters (~6,000 tokens) with a trailing `[...content truncated...]` notice.
- **Main Content Filter**: `onlyMainContent: true` ensures noisy navigation sidebars and footers are omitted before reaching the agent.

---

## 5. Summary of Implementation Tasks

- [ ] **`firecrawl.ts`**: Create the keyless-first Firecrawl HTTP client.
- [ ] **`web-search.ts`**: Use Firecrawl search by default (no API key needed) with DuckDuckGo fallback on rate limit/error.
- [ ] **`fetch.ts`**: Route HTML page requests through Firecrawl `/v1/scrape` with fallback to direct HTTP fetch.
- [ ] **Tests (`tests/web-search.test.ts`)**: Verify keyless requests and fallback behavior.
