/**
 * Provider-neutral prompt-cache types.
 *
 * A stable cache/session identity is reused across turns of one logical
 * conversation so providers with prefix caching can keep a cache warm.
 * It is scoped by provider + model: changing either rotates the identity
 * because cached prefixes are not portable across them.
 */

export type CacheRetention = "short" | "long" | "none";

export type CacheStatus = "hit" | "miss" | "write" | "unknown" | "unsupported";

/**
 * Stable cache/session identity passed through `StreamParams`.
 *
 * `conversationId` is generated once per logical agent conversation and reused
 * across turns and safe retries. `provider` and `modelId` are the canonical
 * route used for the request; changing them rotates the identity because
 * cached prefixes are not portable across providers/models.
 */
export interface CacheIdentity {
  /** Stable per-conversation identifier (UUID by default). Never the raw session secret. */
  conversationId: string;
  /** Canonical provider id (e.g. "antigravity", "openai"). */
  provider: string;
  /** Canonical model id (e.g. "claude-3-7-sonnet"). */
  modelId: string;
}

/** Normalized per-turn token usage with cache breakdown. */
export interface TurnUsage {
  /** Uncached input tokens (prompt minus cache reads). */
  input: number;
  /** Tokens served from cache. */
  cacheRead: number;
  /** Tokens written to cache (0 when the provider does not report it). */
  cacheWrite: number;
  /** Output tokens including reasoning/thinking tokens. */
  output: number;
  /** Reasoning/thinking tokens when reported separately. */
  reasoningTokens?: number;
  /** Total tokens as reported by the provider. */
  totalTokens: number;
  cacheStatus: CacheStatus;
}

/**
 * Build a bounded fingerprint for telemetry. Never the raw cache identity —
 * we only emit a short, opaque tag so logs can correlate hits/misses without
 * leaking the underlying session key.
 */
export function cacheIdentityFingerprint(identity: CacheIdentity): string {
  const conversationTag = identity.conversationId.replace(/-/g, "").slice(0, 8);
  const providerTag = identity.provider.replace(/[^a-zA-Z0-9_-]/g, "").slice(0, 12);
  const modelTag = identity.modelId.replace(/[^a-zA-Z0-9._-]/g, "").slice(0, 24);
  return `${providerTag}:${modelTag}:${conversationTag}`;
}

/**
 * Compare two cache identities. Returns true when they refer to the same
 * logical cache (same provider + model + conversation).
 */
export function isSameCacheIdentity(a: CacheIdentity, b: CacheIdentity): boolean {
  return (
    a.conversationId === b.conversationId &&
    a.provider === b.provider &&
    a.modelId === b.modelId
  );
}
