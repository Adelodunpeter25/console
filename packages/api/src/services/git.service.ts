import { getConsoleApiClient } from "../client";
import type { GitStatusSummary } from "@console/types";

/** Parse one SSE frame (event + data lines) from a raw text chunk buffer. */
function extractSseFrames(buffer: string): { frames: { event: string; data: string }[]; rest: string } {
  const frames: { event: string; data: string }[] = [];
  const parts = buffer.split("\n\n");
  const rest = parts.pop() ?? "";
  for (const part of parts) {
    let event = "message";
    const dataLines: string[] = [];
    for (const line of part.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) dataLines.push(line.slice(5).trimStart());
    }
    frames.push({ event, data: dataLines.join("\n") });
  }
  return { frames, rest };
}

export const gitService = {
  async getDiff(repoPath: string, filePath?: string): Promise<string | null> {
    const res = await getConsoleApiClient().get("/api/git/diff", { params: { repoPath, ...(filePath ? { path: filePath } : {}) } });
    return res.data?.data?.diff ?? null;
  },
  async getStatus(path: string): Promise<GitStatusSummary | null> {
    const res = await getConsoleApiClient().get("/api/git/status", { params: { path } });
    return res.data?.data ?? null;
  },
  /**
   * Subscribe to live git status via GET /api/git/status/watch SSE.
   * Calls onStatus for the initial snapshot and every pushed update.
   * Returns an unsubscribe function. Uses raw fetch since axios can't
   * hold an SSE stream open.
   */
  watchStatus(
    baseUrl: string,
    path: string,
    onStatus: (summary: GitStatusSummary) => void,
    onError?: (message: string) => void,
  ): () => void {
    const controller = new AbortController();
    const url = `${baseUrl}/api/git/status/watch?path=${encodeURIComponent(path)}`;
    (async () => {
      try {
        const res = await fetch(url, {
          headers: { Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (!res.ok || !res.body) {
          onError?.(`Watch stream responded ${res.status}`);
          return;
        }
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const { frames, rest } = extractSseFrames(buffer);
          buffer = rest;
          for (const frame of frames) {
            if (frame.event === "gitStatus" && frame.data) {
              try {
                onStatus(JSON.parse(frame.data) as GitStatusSummary);
              } catch {
                // ignore malformed frame
              }
            }
          }
        }
      } catch (e) {
        if ((e as Error)?.name !== "AbortError") {
          onError?.(e instanceof Error ? e.message : "Watch stream failed");
        }
      }
    })();
    return () => controller.abort();
  },
};