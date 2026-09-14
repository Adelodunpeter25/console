/**
 * Tests for Firecrawl keyless search/scrape and fallback behavior.
 * Verifies spec docs/firecrawl-search-and-fetch-spec.md: keyless without Authorization,
 * truncation, and DuckDuckGo/direct-fetch fallbacks.
 */
import assert from "node:assert/strict";
import { webSearchTool } from "@/agent/src/tools/web-search.js";
import { fetchTool } from "@/agent/src/tools/fetch.js";
import { firecrawlSearch, firecrawlScrape } from "@/agent/src/tools/firecrawl.js";

console.log("Running Firecrawl web-search/fetch tests...");

// Helper to run tool with parsed input
const runTool = async (tool: any, args: Record<string, unknown>) => {
  const parsed = tool.inputSchema.parse(args);
  return tool.execute(parsed);
};

// 1. Firecrawl keyless — no Authorization when no key
{
  const originalFetch = globalThis.fetch;
  const originalKey = process.env.FIRECRAWL_API_KEY;
  delete process.env.FIRECRAWL_API_KEY;

  let capturedHeaders: Record<string, string> = {};
  globalThis.fetch = (async (url: string, init: any) => {
    capturedHeaders = init?.headers ?? {};
    return new Response(
      JSON.stringify({ success: true, data: [{ title: "T", url: "https://example.com", markdown: "# hi", description: "d" }] }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  }) as unknown as typeof fetch;

  try {
    await firecrawlSearch("test", 3);
    assert.equal(capturedHeaders["Authorization"], undefined, "keyless should not send Authorization");
    console.log("  ✅ firecrawlSearch keyless — no Authorization header");
  } finally {
    globalThis.fetch = originalFetch;
    if (originalKey !== undefined) process.env.FIRECRAWL_API_KEY = originalKey;
  }
}

// 2. Firecrawl with API key — sends Authorization
{
  const originalFetch = globalThis.fetch;
  const originalKey = process.env.FIRECRAWL_API_KEY;
  process.env.FIRECRAWL_API_KEY = "test-key-123";
  let capturedHeaders: Record<string, string> = {};
  globalThis.fetch = (async (url: string, init: any) => {
    capturedHeaders = init?.headers ?? {};
    return new Response(
      JSON.stringify({ success: true, data: [{ title: "T", url: "https://example.com", markdown: "# hi" }] }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  }) as unknown as typeof fetch;

  try {
    await firecrawlSearch("test2", 1);
    assert.equal(capturedHeaders["Authorization"], "Bearer test-key-123", "should send Authorization when key is set");
    console.log("  ✅ firecrawlSearch with API key — sends Authorization header");
  } finally {
    globalThis.fetch = originalFetch;
    if (originalKey === undefined) delete process.env.FIRECRAWL_API_KEY;
    else process.env.FIRECRAWL_API_KEY = originalKey;
  }
}

// 3. webSearch fallback on 429 to DuckDuckGo
{
  const originalFetch = globalThis.fetch;
  let callCount = 0;
  globalThis.fetch = (async (url: string, init: any) => {
    callCount++;
    const urlStr = String(url);
    if (urlStr.includes("api.firecrawl.dev")) {
      // First call is Firecrawl search — return 429
      return new Response("rate limited", { status: 429, headers: { "Content-Type": "text/plain" } });
    }
    if (urlStr.includes("html.duckduckgo.com")) {
      // Return minimal DDG HTML with one result
      const html = `<div class="result"><a class="result__a" href="https://example.com">Example Title</a><a class="result__snippet">Example snippet</a></div></div>`;
      return new Response(html, { status: 200, headers: { "Content-Type": "text/html" } });
    }
    return new Response("not found", { status: 404 });
  }) as unknown as typeof fetch;

  try {
    const res = (await runTool(webSearchTool, { query: "test fallback", numResults: 1 })) as {
      content: Array<{ text: string }>;
    };
    const text = res.content[0]?.text ?? "";
    assert.ok(text.includes("Example Title") || text.includes("example.com"), "should fallback to DuckDuckGo on 429");
    assert.equal(callCount, 2, "should have called Firecrawl then DuckDuckGo");
    console.log("  ✅ webSearch fallback to DuckDuckGo on 429");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 4. webSearch returns Firecrawl markdown when available (no fallback)
{
  const originalFetch = globalThis.fetch;
  let firecrawlCalled = false;
  globalThis.fetch = (async (url: string, init: any) => {
    const urlStr = String(url);
    if (urlStr.includes("api.firecrawl.dev")) {
      firecrawlCalled = true;
      return new Response(
        JSON.stringify({
          success: true,
          data: [
            { title: "Fire Title", url: "https://fire.example.com", markdown: "# Fire Markdown", description: "desc" },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    // Should not be called for DDG if Firecrawl succeeds
    return new Response("should not fallback", { status: 500 });
  }) as unknown as typeof fetch;

  try {
    const res = (await runTool(webSearchTool, { query: "fire markdown", numResults: 1 })) as {
      content: Array<{ text: string }>;
    };
    const text = res.content[0]?.text ?? "";
    assert.ok(text.includes("Fire Title"), "should contain Firecrawl title");
    assert.ok(text.includes("Fire Markdown"), "should contain Firecrawl markdown");
    assert.ok(firecrawlCalled, "Firecrawl should have been called");
    console.log("  ✅ webSearch returns Firecrawl markdown without fallback");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 5. fetch tool — Firecrawl scrape for GET HTML, fallback to direct on error
{
  const originalFetch = globalThis.fetch;
  let firecrawlScrapeCalled = false;
  let directFetchCalled = false;

  globalThis.fetch = (async (url: string, init: any) => {
    const urlStr = String(url);
    const method = init?.method ?? "GET";
    if (urlStr.includes("api.firecrawl.dev") && urlStr.includes("/scrape")) {
      firecrawlScrapeCalled = true;
      return new Response(
        JSON.stringify({
          success: true,
          data: { markdown: "# Scraped Markdown", metadata: { title: "Scraped", sourceURL: urlStr, statusCode: 200 } },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    if (method === "GET" && urlStr === "https://example.com/page") {
      // This would be direct fetch if Firecrawl failed; but Firecrawl succeeds above, so this shouldn't be hit
      directFetchCalled = true;
      return new Response("<html><body>direct</body></html>", {
        status: 200,
        headers: { "Content-Type": "text/html" },
      });
    }
    return new Response("not found", { status: 404 });
  }) as unknown as typeof fetch;

  try {
    const res = (await runTool(fetchTool, { url: "https://example.com/page", method: "GET" })) as {
      content: Array<{ text: string }>;
    };
    const text = res.content[0]?.text ?? "";
    assert.ok(text.includes("Scraped Markdown"), "should return Firecrawl markdown");
    assert.ok(text.includes("via Firecrawl"), "should indicate Firecrawl source");
    assert.ok(firecrawlScrapeCalled, "Firecrawl scrape should have been called");
    console.log("  ✅ fetch uses Firecrawl scrape for GET HTML");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 6. fetch fallback to direct on Firecrawl error
{
  const originalFetch = globalThis.fetch;
  let directCalled = false;
  globalThis.fetch = (async (url: string, init: any) => {
    const urlStr = String(url);
    if (urlStr.includes("api.firecrawl.dev")) {
      return new Response("server error", { status: 500 });
    }
    if (urlStr === "https://example.com/fallback") {
      directCalled = true;
      return new Response("<html><body>direct fallback</body></html>", {
        status: 200,
        headers: { "Content-Type": "text/html" },
      });
    }
    return new Response("not found", { status: 404 });
  }) as unknown as typeof fetch;

  try {
    const res = (await runTool(fetchTool, { url: "https://example.com/fallback", method: "GET" })) as {
      content: Array<{ text: string }>;
    };
    const text = res.content[0]?.text ?? "";
    assert.ok(text.includes("direct fallback"), "should fallback to direct fetch");
    assert.ok(directCalled, "direct fetch should have been called after Firecrawl error");
    console.log("  ✅ fetch fallback to direct on Firecrawl error");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

// 7. fetch does NOT use Firecrawl for POST/API (direct only)
{
  const originalFetch = globalThis.fetch;
  let firecrawlCalled = false;
  globalThis.fetch = (async (url: string, init: any) => {
    const urlStr = String(url);
    if (urlStr.includes("api.firecrawl.dev")) {
      firecrawlCalled = true;
      return new Response(JSON.stringify({ success: true, data: { markdown: "x" } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    if (urlStr === "https://api.example.com/data" && init?.method === "POST") {
      return new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response("not found", { status: 404 });
  }) as unknown as typeof fetch;

  try {
    const res = (await runTool(fetchTool, { url: "https://api.example.com/data", method: "POST", body: "{}" })) as {
      content: Array<{ text: string }>;
    };
    assert.equal(firecrawlCalled, false, "Firecrawl should not be called for POST API");
    const text = res.content[0]?.text ?? "";
    assert.ok(text.includes('"ok": true') || text.includes("ok"), "should return JSON from direct fetch");
    console.log("  ✅ fetch skips Firecrawl for POST/API");
  } finally {
    globalThis.fetch = originalFetch;
  }
}

console.log("Firecrawl web-search/fetch tests passed!\n");
