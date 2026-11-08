// Server discovery: which Console backend this app talks to. The desktop's
// `HttpTransport` resolves `CONSOLE_BACKEND_URL` from the environment,
// then reads the desktop's active environment or paired server store,
// and falls back to localhost:3000.

import { backend } from "./backend";

const DESKTOP_DEFAULT = "http://localhost:3000";

/// The backend base URL for this run. Reads `CONSOLE_BACKEND_URL` once at
/// startup (the env does not change under a running app); a trailing slash
/// is trimmed so URL joins stay predictable. Falls back to the desktop state's
/// active paired server.
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
  try {
    const active = backend.activeServerUrl();
    if (active != null && active.trim() !== "") {
      return active.trim().replace(/\/+$/, "");
    }
  } catch {
    // Bridge call failed; fall through.
  }
  return DESKTOP_DEFAULT;
}
