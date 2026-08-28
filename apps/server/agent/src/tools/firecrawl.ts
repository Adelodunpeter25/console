/**
 * Firecrawl HTTP client — keyless-first.
 * Uses https://api.firecrawl.dev/v1 without Authorization by default;
 * if FIRECRAWL_API_KEY is set, sends Bearer token. Supports self-hosted
 * via FIRECRAWL_BASE_URL.
 */

const FIRECRAWL_BASE_URL = process.env.FIRECRAWL_BASE_URL?.replace(/\/+$/, "") || "https://api.firecrawl.dev/v1";
const FIRECRAWL_API_KEY = process.env.FIRECRAWL_API_KEY;

const MAX_MARKDOWN_CHARS = 25_000;
const TRUNCATE_NOTICE = "\n\n[...content truncated...]";

function truncateMarkdown(text: string): string {
  if (text.length <= MAX_MARKDOWN_CHARS) return text;
  return text.slice(0, MAX_MARKDOWN_CHARS) + TRUNCATE_NOTICE;
}

function buildHeaders(): Record<string, string> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (FIRECRAWL_API_KEY) {
    headers["Authorization"] = `Bearer ${FIRECRAWL_API_KEY}`;
  }
  return headers;
}

export interface FirecrawlSearchResult {
  title?: string;
  url?: string;
  markdown?: string;
  description?: string;
}

export interface FirecrawlSearchResponse {
  success: boolean;
  data?: FirecrawlSearchResult[];
  error?: string;
}

export async function firecrawlSearch(
  query: string,
  limit = 5,
  signal?: AbortSignal,
): Promise<FirecrawlSearchResponse> {
  const response = await fetch(`${FIRECRAWL_BASE_URL}/search`, {
    method: "POST",
    headers: buildHeaders(),
    body: JSON.stringify({
      query,
      limit,
      scrapeOptions: { formats: ["markdown"], onlyMainContent: true },
    }),
    signal,
  });

  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new Error(`Firecrawl search error: HTTP ${response.status} ${body.slice(0, 500)}`);
  }

  const json = (await response.json()) as FirecrawlSearchResponse;
  // Truncate markdown per result
  if (json.data) {
    for (const r of json.data) {
      if (r.markdown) r.markdown = truncateMarkdown(r.markdown);
    }
  }
  return json;
}

export interface FirecrawlScrapeResponse {
  success: boolean;
  data?: {
    markdown?: string;
    metadata?: {
      title?: string;
      sourceURL?: string;
      statusCode?: number;
    };
  };
  error?: string;
}

export async function firecrawlScrape(
  url: string,
  signal?: AbortSignal,
): Promise<FirecrawlScrapeResponse> {
  const response = await fetch(`${FIRECRAWL_BASE_URL}/scrape`, {
    method: "POST",
    headers: buildHeaders(),
    body: JSON.stringify({
      url,
      formats: ["markdown"],
      onlyMainContent: true,
    }),
    signal,
  });

  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new Error(`Firecrawl scrape error: HTTP ${response.status} ${body.slice(0, 500)}`);
  }

  const json = (await response.json()) as FirecrawlScrapeResponse;
  if (json.data?.markdown) {
    json.data.markdown = truncateMarkdown(json.data.markdown);
  }
  return json;
}

export function isRetryableFirecrawlError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err);
  // 429 rate limit, network failures, 5xx
  if (msg.includes("429") || msg.includes("Rate limit")) return true;
  if (msg.includes("HTTP 5") || msg.includes("HTTP 429")) return true;
  if (msg.includes("fetch failed") || msg.includes("network")) return true;
  return false;
}
