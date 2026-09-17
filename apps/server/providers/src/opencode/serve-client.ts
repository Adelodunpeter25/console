/**
 * Minimal HTTP client for a local `opencode serve` sidecar (opencode v2 API).
 *
 * The sidecar is owned by `console start`/`console stop` (see
 * apps/cli/opencode-sidecar.ts), which persists { port, password } to
 * `<console-dir>/opencode-serve.json`. This client only *adopts* that state —
 * it never spawns processes.
 *
 * One serve process multiplexes unlimited sessions; each Console conversation
 * maps to one opencode session id (created lazily, reused per conversation).
 * Tools are NOT forwarded: opencode runs its own built-in tools inside its
 * workspace, so turns come back as text (+ usage). Console-side tool calls
 * never fire for this provider in v1.
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import { getConsoleStorageDir } from "@/agent/src/session/apppaths.js";
import { parseSse } from "@/providers/src/shared/sse-parser.js";
import type { TurnUsage } from "@console/types";

export const OPENCODE_SIDECAR_MISSING_HINT =
  "opencode sidecar is not running — run `console start` (or `opencode serve --port 4096`) and retry";

export interface SidecarRef {
  baseUrl: string;
  password: string;
}

export async function loadSidecarRef(): Promise<SidecarRef | null> {
  const portEnv = process.env.OPENCODE_SERVE_PORT;
  const passwordEnv = process.env.OPENCODE_SERVE_PASSWORD;
  if (portEnv && passwordEnv) {
    return { baseUrl: `http://127.0.0.1:${portEnv}`, password: passwordEnv };
  }
  try {
    const raw = await fs.readFile(path.join(getConsoleStorageDir(), "opencode-serve.json"), "utf-8");
    const parsed = JSON.parse(raw) as { port?: number; password?: string };
    if (typeof parsed.port === "number" && typeof parsed.password === "string") {
      return { baseUrl: `http://127.0.0.1:${parsed.port}`, password: parsed.password };
    }
    return null;
  } catch {
    return null;
  }
}

function authHeaders(ref: SidecarRef, cwd?: string): Record<string, string> {
  return {
    Authorization: `Basic ${Buffer.from(`opencode:${ref.password}`).toString("base64")}`,
    ...(cwd ? { "x-opencode-directory": cwd } : {}),
  };
}

async function serveFetch(ref: SidecarRef, reqPath: string, init: RequestInit, cwd?: string): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(`${ref.baseUrl}${reqPath}`, {
      ...init,
      headers: { ...(init.headers as Record<string, string> | undefined), ...authHeaders(ref, cwd) },
    });
  } catch (error) {
    throw new Error(`${OPENCODE_SIDECAR_MISSING_HINT} (${error})`);
  }
  if (response.status === 401 || response.status === 403) {
    throw new Error(`${OPENCODE_SIDECAR_MISSING_HINT} (serve rejected credentials)`);
  }
  return response;
}

/** Normalize `opencode/<id>` display ids to the bare Zen model id. */
export function normalizeServeModelId(modelId: string): string {
  return modelId.startsWith("opencode/") ? modelId.slice("opencode/".length) : modelId;
}

export async function createServeSession(ref: SidecarRef, cwd: string, modelId: string): Promise<string> {
  const response = await serveFetch(
    ref,
    "/api/session",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        model: { providerID: "opencode", id: normalizeServeModelId(modelId) },
      }),
    },
    cwd,
  );
  if (!response.ok) {
    throw new Error(`opencode serve create session failed (${response.status}): ${await response.text()}`);
  }
  const payload = (await response.json()) as { data?: { id?: string } };
  const id = payload.data?.id;
  if (!id) throw new Error("opencode serve create session returned no id");
  return id;
}

export async function postServePrompt(
  ref: SidecarRef,
  sessionId: string,
  text: string,
  cwd: string,
): Promise<void> {
  const response = await serveFetch(
    ref,
    `/api/session/${sessionId}/prompt`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    },
    cwd,
  );
  if (!response.ok) {
    throw new Error(`opencode serve prompt failed (${response.status}): ${await response.text()}`);
  }
}

export async function interruptServeSession(ref: SidecarRef, sessionId: string, cwd: string): Promise<void> {
  await serveFetch(ref, `/api/session/${sessionId}/interrupt`, { method: "POST" }, cwd).catch(() => {});
}

interface ServeEvent {
  type?: string;
  data?: {
    sessionID?: string;
    delta?: string;
    finish?: string;
    tokens?: { input?: number; output?: number; reasoning?: number; cache?: { read?: number; write?: number } };
    message?: string;
    error?: { message?: string } | string;
  };
}

type ServeTokens = NonNullable<NonNullable<ServeEvent["data"]>["tokens"]>;

export function serveTokensToUsage(tokens: ServeTokens | undefined): TurnUsage {
  const input = tokens?.input ?? 0;
  const cacheRead = tokens?.cache?.read ?? 0;
  const cacheWrite = tokens?.cache?.write ?? 0;
  const output = tokens?.output ?? 0;
  const reasoning = tokens?.reasoning;
  return {
    input: Math.max(0, input - cacheRead),
    cacheRead,
    cacheWrite,
    output,
    ...(reasoning !== undefined ? { reasoningTokens: reasoning } : {}),
    totalTokens: input + output + (reasoning ?? 0),
    cacheStatus: cacheRead > 0 ? "hit" : "miss",
  };
}

/** Pure event→delta mapping, unit-tested without a live server. */
export function mapServeEvent(
  event: ServeEvent,
  sessionId: string,
): { delta?: { type: "text"; text: string } | { type: "thinking"; text: string }; done?: boolean; usage?: TurnUsage; error?: string } {
  if (event.data?.sessionID && event.data.sessionID !== sessionId) return {};
  switch (event.type) {
    case "session.text.delta":
      return event.data?.delta ? { delta: { type: "text", text: event.data.delta } } : {};
    case "session.reasoning.delta":
      return event.data?.delta ? { delta: { type: "thinking", text: event.data.delta } } : {};
    case "session.step.ended":
      return event.data?.tokens ? { usage: serveTokensToUsage(event.data.tokens) } : {};
    case "session.execution.succeeded":
      return { done: true };
    case "session.execution.failed":
    case "session.error":
      return {
        error:
          typeof event.data?.error === "string"
            ? event.data.error
            : (event.data?.error?.message ?? event.data?.message ?? "opencode execution failed"),
      };
    default:
      return {};
  }
}

/**
 * Subscribe to the global serve event stream and yield mapped deltas for one
 * session until its execution succeeds/fails. The caller must POST the prompt
 * while (or before) this generator runs — events arrive asynchronously.
 */
export async function* streamServeSession(
  ref: SidecarRef,
  sessionId: string,
  cwd: string,
  signal?: AbortSignal,
): AsyncGenerator<
  { type: "text"; text: string } | { type: "thinking"; text: string } | { type: "usage"; usage: TurnUsage }
> {
  let response: Response;
  try {
    response = await fetch(`${ref.baseUrl}/api/event`, {
      headers: { Accept: "text/event-stream", ...authHeaders(ref, cwd) },
      signal,
    });
  } catch (error) {
    throw new Error(`${OPENCODE_SIDECAR_MISSING_HINT} (${error})`);
  }
  if (!response.ok) throw new Error(`opencode serve event stream failed (${response.status})`);

  let pendingUsage: TurnUsage | undefined;
  try {
    for await (const event of parseSse<ServeEvent>(response)) {
      if (signal?.aborted) {
        await interruptServeSession(ref, sessionId, cwd);
        break;
      }
      const mapped = mapServeEvent(event, sessionId);
      if (mapped.error) throw new Error(mapped.error);
      if (mapped.delta) yield mapped.delta;
      if (mapped.usage) pendingUsage = mapped.usage;
      if (mapped.done) break;
    }
  } finally {
    // Release the SSE connection. parseSse may have exited early (session
    // done/failed) while the server keeps the stream open with heartbeats.
    try {
      await response.body?.cancel();
    } catch {
      // already closed
    }
  }

  yield {
    type: "usage",
    usage: pendingUsage ?? {
      input: 0,
      cacheRead: 0,
      cacheWrite: 0,
      output: 0,
      totalTokens: 0,
      cacheStatus: "unknown",
    },
  };
}
