import type { AgentSessionEvent } from "@console/types";

/**
 * Incremental SSE parser for XMLHttpRequest streaming.
 *
 * The server emits newline-delimited `data: <json>` frames. XHR delivers
 * arbitrary chunk boundaries, so this parser keeps a partial-line buffer and
 * only emits complete JSON frames. Incomplete trailing lines are held for the
 * next chunk (and dropped on stream end).
 */
export function createSseParser() {
  let buffer = "";

  return {
    /** Feed a raw chunk of response text; returns every complete event parsed. */
    push(chunk: string): AgentSessionEvent[] {
      buffer += chunk;
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      const events: AgentSessionEvent[] = [];
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed.startsWith("data: ")) continue;
        try {
          const event = JSON.parse(trimmed.slice(6)) as AgentSessionEvent;
          events.push(event);
        } catch {
          // Ignore JSON parse errors for incomplete lines
        }
      }
      return events;
    },

    /** Discard any partial trailing frame (stream finished). */
    flush(): void {
      buffer = "";
    },
  };
}
