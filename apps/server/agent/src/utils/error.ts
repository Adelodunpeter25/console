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
