/**
 * Normalize a user-entered backend URL.
 * Trims whitespace, strips trailing slashes and defaults the scheme to http://
 * (matching how LAN backend endpoints are typically typed).
 * Returns null when nothing usable remains.
 */
export function normalizeBackendUrl(input: string): string | null {
  let url = input.trim().replace(/\/+$/, "");
  if (!url) return null;
  if (!url.startsWith("http://") && !url.startsWith("https://")) {
    url = `http://${url}`;
  }
  return url;
}

/** Host portion of a normalized URL for compact display ("moonbase.tail9f3a.ts.net"). */
export function urlHost(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}
