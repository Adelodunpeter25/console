/**
 * Robust error extraction utility for AI SDK, HTTP, and provider error structures.
 */
export function extractErrorMessage(err: unknown): string {
  if (!err) return "An unknown error occurred.";
  if (typeof err === "string") return err;

  if (typeof err === "object") {
    const anyErr = err as any;

    // 1. Dig into nested AI SDK error structures (AI_RetryError -> lastError / cause)
    const possibleBodies = [
      anyErr.responseBody,
      anyErr.lastError?.responseBody,
      anyErr.cause?.responseBody,
      anyErr.data?.error?.message,
      anyErr.data?.message,
    ];

    for (const body of possibleBodies) {
      if (!body) continue;
      if (typeof body === "string") {
        try {
          const parsed = JSON.parse(body);
          if (parsed?.error?.message && typeof parsed.error.message === "string") {
            return parsed.error.message;
          }
          if (parsed?.message && typeof parsed.message === "string") {
            return parsed.message;
          }
        } catch {
          if (body.trim().length > 0) return body.trim();
        }
      } else if (typeof body === "object" && body !== null) {
        if (body.message) return String(body.message);
      }
    }

    // 2. Check lastError or cause message
    if (anyErr.lastError?.message && typeof anyErr.lastError.message === "string") {
      return anyErr.lastError.message;
    }
    if (anyErr.cause?.message && typeof anyErr.cause.message === "string") {
      return anyErr.cause.message;
    }

    // 3. Check direct error message
    if (anyErr.message && typeof anyErr.message === "string") {
      // If message itself is serialized JSON, parse it
      try {
        const parsed = JSON.parse(anyErr.message);
        if (parsed?.error?.message) return parsed.error.message;
        if (parsed?.message) return parsed.message;
      } catch {}
      return anyErr.message;
    }
  }

  return String(err);
}

function errorText(error: unknown): string {
  const message = extractErrorMessage(error);
  if (error instanceof Error) {
    const stderr =
      "stderr" in error && typeof (error as { stderr?: unknown }).stderr === "string"
        ? (error as { stderr: string }).stderr
        : "";
    return `${message}\n${stderr}`.toLowerCase();
  }
  return message.toLowerCase();
}

/**
 * Whether a git failure just means "not a git repository".
 * Centralized here so `GitService` (and future callers) share one predicate
 * instead of each matching on stderr text.
 */
export function isNotGitRepositoryError(error: unknown): boolean {
  const message = errorText(error);
  return (
    message.includes("not a git repository") ||
    message.includes("not a repository") ||
    message.includes("no git repository")
  );
}

const OVERFLOW_MESSAGE_PATTERNS = [
  "input token count exceeds",
  "context_length_exceeded",
  "prompt_too_long",
  "maximum context length",
  "context window",
  "context limit",
  "too many tokens",
];

/**
 * Whether a provider failure means the request exceeded the model's context
 * window (agent-loop recovers with emergency compaction + one retry).
 * Matches overflow signatures across Antigravity/CCA, Anthropic, OpenAI/Codex,
 * and OpenCode error shapes.
 */
export function isContextOverflowError(error: unknown): boolean {
  const message = errorText(error);
  if (OVERFLOW_MESSAGE_PATTERNS.some((pattern) => message.includes(pattern))) {
    return true;
  }
  // Bare HTTP 400/413 only counts with a size-related keyword — a 400 from a
  // schema or auth problem must not trigger destructive recovery.
  return (
    /\b(400|413)\b/.test(message) &&
    /token|context|exceed|too long|too large|maximum|length|large|big/.test(message)
  );
}
