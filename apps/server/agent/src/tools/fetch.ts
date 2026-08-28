import { z } from "zod";
import type { AgentTool } from "@/agent/src/types/index.js";
import { firecrawlScrape, isRetryableFirecrawlError } from "./firecrawl.js";

const inputSchema = z.object({
  url: z.string().url().describe("The URL to fetch"),
  method: z
    .enum(["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"])
    .optional()
    .default("GET")
    .describe("HTTP method"),
  headers: z.record(z.string()).optional().describe("HTTP request headers as key-value pairs"),
  body: z.string().optional().describe("Request body as a string (for POST/PUT/PATCH)"),
  timeoutMs: z
    .number()
    .int()
    .min(1000)
    .max(60_000)
    .optional()
    .default(15_000)
    .describe("Request timeout in milliseconds (default 15s)"),
  maxBytes: z
    .number()
    .int()
    .min(1024)
    .max(5 * 1024 * 1024)
    .optional()
    .default(512 * 1024)
    .describe("Maximum response body size in bytes (default 512KB)"),
  returnHeaders: z
    .boolean()
    .optional()
    .default(false)
    .describe("Include response headers in the output"),
});

type Input = z.infer<typeof inputSchema>;

/**
 * Naively strip HTML tags and collapse whitespace for cleaner LLM context.
 * Not a full HTML parser — good enough for plain-text extraction.
 */
function htmlToText(html: string): string {
  return html
    .replace(/<style[^>]*>[\s\S]*?<\/style>/gi, "")
    .replace(/<script[^>]*>[\s\S]*?<\/script>/gi, "")
    .replace(/<[^>]+>/g, " ")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/\s{2,}/g, " ")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export const fetchTool: AgentTool<typeof inputSchema> = {
  name: "fetch",
  description: `Fetch content from a URL.`,
  inputSchema,
  execute: async (args: Input, parentSignal?: AbortSignal): Promise<unknown> => {
    const controller = new AbortController();
    const timeoutHandle = setTimeout(() => controller.abort(), args.timeoutMs);

    const onParentAbort = () => controller.abort();
    if (parentSignal) {
      if (parentSignal.aborted) {
        onParentAbort();
      } else {
        parentSignal.addEventListener("abort", onParentAbort, { once: true });
      }
    }

    const isParentAborted = () => parentSignal?.aborted ?? false;

    // Keyless Firecrawl for HTML GETs (spec 3.2) — only for web pages, not APIs/POSTs
    const isGet = (args.method ?? "GET") === "GET" && !args.body;
    const isLikelyApi =
      args.url.includes("/api/") ||
      args.url.endsWith(".json") ||
      (args.headers && Object.values(args.headers).some((v) => String(v).includes("application/json")));
    if (isGet && !isLikelyApi) {
      try {
        const fc = await firecrawlScrape(args.url, controller.signal);
        if (fc.success && fc.data?.markdown) {
          clearTimeout(timeoutHandle);
          if (parentSignal) parentSignal.removeEventListener("abort", onParentAbort);
          const sections: string[] = [
            `URL: ${args.url}`,
            `Status: ${fc.data.metadata?.statusCode ?? 200} OK (via Firecrawl)`,
            `Title: ${fc.data.metadata?.title ?? ""}`,
            "",
            "Body (markdown):",
            fc.data.markdown,
          ];
          if (args.returnHeaders) {
            sections.splice(3, 0, `SourceURL: ${fc.data.metadata?.sourceURL ?? args.url}`);
          }
          return {
            content: [{ type: "text", text: sections.join("\n") }],
          };
        }
      } catch (fcErr: unknown) {
        const err = fcErr as Error;
        if (err.name === "AbortError") {
          clearTimeout(timeoutHandle);
          if (parentSignal) parentSignal.removeEventListener("abort", onParentAbort);
          if (isParentAborted()) {
            return {
              content: [{ type: "text", text: `Request cancelled by user abort — ${args.url}` }],
              isError: true,
            };
          }
          return {
            content: [{ type: "text", text: `Error: Request timed out after ${args.timeoutMs}ms — ${args.url}` }],
            isError: true,
          };
        }
        // Fallback to direct fetch on any Firecrawl error (spec: Error / Rate Limit -> direct)
        // Only abort fallback if it's a non-retryable and we want to surface? Spec says always fallback, so continue.
      }
    }

    let response: Response;
    try {
      response = await fetch(args.url, {
        method: args.method,
        headers: args.headers,
        body: args.body,
        signal: controller.signal,
      });
    } catch (err: unknown) {
      clearTimeout(timeoutHandle);
      if (parentSignal) parentSignal.removeEventListener("abort", onParentAbort);
      const error = err as Error;
      if (error.name === "AbortError") {
        if (isParentAborted()) {
          return {
            content: [
              {
                type: "text",
                text: `Request cancelled by user abort — ${args.url}`,
              },
            ],
            isError: true,
          };
        }
        return {
          content: [
            {
              type: "text",
              text: `Error: Request timed out after ${args.timeoutMs}ms — ${args.url}`,
            },
          ],
          isError: true,
        };
      }
      return {
        content: [{ type: "text", text: `Error: ${error.message}` }],
        isError: true,
      };
    } finally {
      clearTimeout(timeoutHandle);
      if (parentSignal) parentSignal.removeEventListener("abort", onParentAbort);
    }

    // Collect response headers summary
    const headersText = args.returnHeaders
      ? Array.from(response.headers.entries())
          .map(([k, v]) => `  ${k}: ${v}`)
          .join("\n")
      : "";

    // Read body with size cap
    const contentType = response.headers.get("content-type") ?? "";
    let bodyText: string;
    try {
      const buffer = await response.arrayBuffer();
      const bytes = Buffer.from(buffer);

      if (bytes.length > args.maxBytes) {
        const truncated = bytes.slice(0, args.maxBytes).toString("utf-8");
        bodyText =
          truncated +
          `\n\n[... response truncated: ${bytes.length} bytes total, showing first ${args.maxBytes} bytes ...]`;
      } else {
        bodyText = bytes.toString("utf-8");
      }
    } catch (err: unknown) {
      return {
        content: [{ type: "text", text: `Error reading response body: ${(err as Error).message}` }],
        isError: true,
      };
    }

    // Format body based on content type
    let formattedBody: string;
    if (contentType.includes("application/json") || contentType.includes("+json")) {
      try {
        const parsed = JSON.parse(bodyText);
        formattedBody = JSON.stringify(parsed, null, 2);
      } catch {
        formattedBody = bodyText;
      }
    } else if (contentType.includes("text/html")) {
      formattedBody = htmlToText(bodyText);
    } else {
      formattedBody = bodyText;
    }

    const sections: string[] = [
      `URL: ${args.url}`,
      `Status: ${response.status} ${response.statusText}`,
      `Content-Type: ${contentType}`,
    ];
    if (args.returnHeaders && headersText) {
      sections.push(`Headers:\n${headersText}`);
    }
    sections.push("", "Body:", formattedBody);

    const isError = response.status >= 400;
    return {
      content: [{ type: "text", text: sections.join("\n") }],
      isError,
    };
  },
};
