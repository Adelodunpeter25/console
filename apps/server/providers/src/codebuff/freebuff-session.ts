/**
 * Freebuff session manager — replicates the freebuff CLI's session handshake
 * (`cli/src/hooks/use-freebuff-session.ts`) so API calls pass the server's
 * `free_mode_cli_required` gate:
 *
 *   1. POST {base}/api/v1/freebuff/session    + header `x-freebuff-model`
 *         → { status: "active", instanceId, model, expiresAt, ... }
 *   2. Every chat-completions request carries `freebuff_instance_id` in
 *      codebuff_metadata (exactly what the CLI does).
 *
 * Sessions are model-bound and are consumed from the user's daily free-tier
 * quota, so we cache one active session per model and reuse it until it
 * expires instead of burning a new session every request.
 */
import { CODEBUFF_API_URL } from "./constants.js";
import type { CodebuffCredential } from "./creds.js";

/** Wire response of POST /api/v1/freebuff/session (subset we need). */
export interface FreebuffSessionActive {
  status: "active";
  accessTier: "full" | "limited";
  instanceId: string;
  model: string;
  admittedAt: string;
  expiresAt: string;
  remainingMs: number;
}

export type FreebuffSessionResult =
  | { ok: true; session: FreebuffSessionActive }
  | {
      ok: false;
      statusCode: number;
      reason: string;
      message?: string;
      /** e.g. "rate_limited", "country_blocked", "banned", "model_locked". */
      detail?: string;
    };

/** Cached sessions keyed by `${authToken}::${model}`. */
const cachedSessions = new Map<string, FreebuffSessionActive>();

function cacheKey(credential: CodebuffCredential, model: string): string {
  return `${credential.authToken}::${model}`;
}

/** Returns a live cached session for the model, or undefined if stale/none. */
function getCachedActiveSession(
  credential: CodebuffCredential,
  model: string,
): FreebuffSessionActive | undefined {
  const cached = cachedSessions.get(cacheKey(credential, model));
  if (!cached) return undefined;
  // Allow a small skew (5s) so we don't race the server's expiry clock.
  if (Date.parse(cached.expiresAt) - 5_000 <= Date.now()) {
    cachedSessions.delete(cacheKey(credential, model));
    return undefined;
  }
  return cached;
}

/**
 * Ensure an active freebuff session exists for the given model, creating one
 * when necessary (POST /api/v1/freebuff/session with the `x-freebuff-model`
 * header). Returns the instanceId to inject into chat-completions metadata.
 */
export async function ensureFreebuffSession(
  credential: CodebuffCredential,
  model: string,
): Promise<FreebuffSessionResult> {
  const cached = getCachedActiveSession(credential, model);
  if (cached) {
    return { ok: true, session: cached };
  }

  const response = await fetch(`${CODEBUFF_API_URL}/freebuff/session`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${credential.authToken}`,
      "x-freebuff-model": model,
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    const raw = await response.text().catch(() => "");
    let reason = "free_mode_required";
    let detail: string | undefined;
    let message: string | undefined;
    try {
      const body = JSON.parse(raw) as {
        error?: string;
        message?: string;
        status?: string;
      };
      detail = body.error;
      message = body.message;
      // Non-200 codes don't always carry a `status` field; fall back to the
      // error code / HTTP status.
      reason =
        body.status ??
        (response.status === 429
          ? "rate_limited"
          : response.status === 403
            ? "country_blocked"
            : "free_mode_required");
    } catch {
      reason =
        response.status === 429
          ? "rate_limited"
          : response.status === 403
            ? "country_blocked"
            : "free_mode_required";
    }
    return { ok: false, statusCode: response.status, reason, message, detail };
  }

  const body = (await response.json()) as Record<string, unknown>;

  // 409 model_locked / model_unavailable responses arrive with 200 in some
  // paths, so handle the inline-status shape too.
  if (body.status === "active") {
    const session = body as unknown as FreebuffSessionActive;
    cachedSessions.set(cacheKey(credential, model), session);
    return { ok: true, session };
  }

  return {
    ok: false,
    statusCode: 200,
    reason:
      typeof body.status === "string" ? (body.status as string) : "unknown",
    detail: typeof body.error === "string" ? body.error : undefined,
    message: typeof body.message === "string" ? body.message : undefined,
  };
}

/** Drop a cached session (e.g. user ended the session or switched models). */
export function invalidateFreebuffSession(
  credential: CodebuffCredential,
  model: string,
): void {
  cachedSessions.delete(cacheKey(credential, model));
}