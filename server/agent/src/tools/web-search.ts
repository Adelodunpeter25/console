import { z } from "zod";
import type { AgentTool } from "../types/index.js";

const inputSchema = z.object({
  query: z.string().describe("The search query"),
  numResults: z
    .number()
    .int()
    .min(1)
    .max(20)
    .optional()
    .default(5)
    .describe("Number of results to return (1–20, default 5)"),
  searchEngine: z
    .enum(["duckduckgo", "brave"])
    .optional()
    .default("duckduckgo")
    .describe("Search engine to use"),
});

type Input = z.infer<typeof inputSchema>;

interface SearchResult {
  title: string;
  url: string;
  snippet: string;
}

// ---------------------------------------------------------------------------
// DuckDuckGo instant answer / HTML scraper (no API key required)
// ---------------------------------------------------------------------------

async function searchDuckDuckGo(
  query: string,
  numResults: number,
  signal: AbortSignal,
): Promise<SearchResult[]> {
  // Use DuckDuckGo's lite HTML endpoint — no JS required, no API key
  const url = `https://html.duckduckgo.com/html/?q=${encodeURIComponent(query)}`;
  const response = await fetch(url, {
    headers: {
      "User-Agent":
        "Mozilla/5.0 (compatible; AgentHarness/1.0; +https://github.com/console-agent)",
      Accept: "text/html",
    },
    signal,
  });

  if (!response.ok) {
    throw new Error(`DuckDuckGo returned ${response.status}`);
  }

  const html = await response.text();

  // Parse results from the HTML
  // DDG lite wraps results in <div class="result">
  const results: SearchResult[] = [];
  const resultBlockRe = /<div class="result[^"]*"[^>]*>([\s\S]*?)<\/div>\s*<\/div>/g;
  const titleUrlRe = /<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>([\s\S]*?)<\/a>/;
  const snippetRe = /<a[^>]+class="result__snippet"[^>]*>([\s\S]*?)<\/a>/;

  let match: RegExpExecArray | null;
  while ((match = resultBlockRe.exec(html)) !== null && results.length < numResults) {
    const block = match[1]!;
    const titleUrlMatch = titleUrlRe.exec(block);
    const snippetMatch = snippetRe.exec(block);

    if (!titleUrlMatch) continue;

    const rawUrl = titleUrlMatch[1]!;
    const title = titleUrlMatch[2]!
      .replace(/<[^>]+>/g, "")
      .replace(/&amp;/g, "&")
      .replace(/&lt;/g, "<")
      .replace(/&gt;/g, ">")
      .replace(/&#x27;/g, "'")
      .trim();
    const snippet = snippetMatch
      ? snippetMatch[1]!
          .replace(/<[^>]+>/g, "")
          .replace(/&amp;/g, "&")
          .replace(/&nbsp;/g, " ")
          .trim()
      : "";

    // DDG lite URLs are redirects — try to extract the real URL
    let finalUrl = rawUrl;
    try {
      const parsed = new URL(rawUrl, "https://html.duckduckgo.com");
      const uddg = parsed.searchParams.get("uddg");
      if (uddg) finalUrl = decodeURIComponent(uddg);
    } catch {
      // keep raw url
    }

    if (title && finalUrl) {
      results.push({ title, url: finalUrl, snippet });
    }
  }

  return results;
}

// ---------------------------------------------------------------------------
// Brave Search (free tier — requires BRAVE_SEARCH_API_KEY env var)
// ---------------------------------------------------------------------------

async function searchBrave(
  query: string,
  numResults: number,
  signal: AbortSignal,
): Promise<SearchResult[]> {
  const apiKey = process.env.BRAVE_SEARCH_API_KEY;
  if (!apiKey) {
    throw new Error(
      "Brave Search requires a BRAVE_SEARCH_API_KEY environment variable. " +
        "Get a free key at https://api.search.brave.com/register",
    );
  }

  const url = `https://api.search.brave.com/res/v1/web/search?q=${encodeURIComponent(query)}&count=${numResults}`;
  const response = await fetch(url, {
    headers: {
      "Accept": "application/json",
      "Accept-Encoding": "gzip",
      "X-Subscription-Token": apiKey,
    },
    signal,
  });

  if (!response.ok) {
    throw new Error(`Brave Search returned ${response.status}: ${response.statusText}`);
  }

  const data = (await response.json()) as {
    web?: {
      results?: Array<{
        title?: string;
        url?: string;
        description?: string;
      }>;
    };
  };

  return (data.web?.results ?? []).slice(0, numResults).map((r) => ({
    title: r.title ?? "",
    url: r.url ?? "",
    snippet: r.description ?? "",
  }));
}

// ---------------------------------------------------------------------------
// Tool definition
// ---------------------------------------------------------------------------

export const webSearchTool: AgentTool<typeof inputSchema> = {
  name: "webSearch",
  description: `Search the web and return a list of relevant results.
Returns result titles, URLs, and snippets.
Use this to find up-to-date information, documentation, package details, or to research topics.
After finding URLs, use the fetch tool to read the full content of any page.
DuckDuckGo requires no API key. Brave Search requires BRAVE_SEARCH_API_KEY env var (free tier available).`,
  inputSchema,
  execute: async (args: Input): Promise<unknown> => {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15_000);

    let results: SearchResult[];
    try {
      if (args.searchEngine === "brave") {
        results = await searchBrave(args.query, args.numResults, controller.signal);
      } else {
        results = await searchDuckDuckGo(args.query, args.numResults, controller.signal);
      }
    } catch (err: unknown) {
      clearTimeout(timeout);
      const error = err as Error;
      if (error.name === "AbortError") {
        return {
          content: [{ type: "text", text: "Error: Web search timed out after 15s." }],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error: ${error.message}` }],
        isError: true,
      };
    } finally {
      clearTimeout(timeout);
    }

    if (results.length === 0) {
      return {
        content: [
          {
            type: "text",
            text: `No results found for: "${args.query}"`,
          },
        ],
      };
    }

    const lines: string[] = [
      `Web search results for: "${args.query}" (${results.length} results)\n`,
    ];
    for (let i = 0; i < results.length; i++) {
      const r = results[i]!;
      lines.push(`[${i + 1}] ${r.title}`);
      lines.push(`    URL: ${r.url}`);
      if (r.snippet) lines.push(`    ${r.snippet}`);
      lines.push("");
    }

    return {
      content: [{ type: "text", text: lines.join("\n") }],
    };
  },
};
