// Server discovery: which Console backend this app talks to. The desktop's
// `HttpTransport` resolves `CONSOLE_BACKEND_URL` from the environment and
// falls back to localhost:3000; the nsh app resolves the same way so both
// apps hit the same server with the same precedence.

const DESKTOP_DEFAULT = "http://localhost:3000";

/// The backend base URL for this run. Reads `CONSOLE_BACKEND_URL` once at
/// startup (the env does not change under a running app); a trailing slash
/// is trimmed so URL joins stay predictable.
export function resolveBackendUrl(): string {
  try {
    const env =
      (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env;
    const fromEnv = env?.["CONSOLE_BACKEND_URL"]?.trim();
    if (fromEnv != null && fromEnv !== "") {
      return fromEnv.replace(/\/+$/, "");
    }
  } catch {
    // No process/env in this runtime (some nsh hosts); fall through.
  }
  return DESKTOP_DEFAULT;
}
