import { Platform } from "react-native";

let initialized = false;

/**
 * Initializes Nitro Fetch (powered by Cronet on Android and URLSession on iOS)
 * as the global fetch implementation for accelerated native networking, HTTP/3,
 * connection pre-warming, and direct native JSI bindings without JS bridge overhead.
 *
 * Cleartext exception: plain `http://` URLs (LAN Console server, no TLS) bypass
 * Nitro/Cronet and use RN's default fetch (OkHttp), which honors the Android
 * Network Security Config (`@xml/network_security_config`). Cronet manages its
 * own stack and still rejects cleartext after the Expo 54 -> 57 upgrade, which
 * broke LAN connections. This restores Expo 54 behavior for http while keeping
 * Nitro speed for https.
 */
function getFetchUrl(input: unknown): string {
  if (typeof input === "string") return input;
  try {
    const url = (input as { url?: unknown })?.url;
    if (typeof url === "string") return url;
    const href = (input as { href?: unknown })?.href;
    if (typeof href === "string") return href;
  } catch {
    // ignore and fall through to Nitro
  }
  return "";
}

export function initNitroFetch(): void {
  if (initialized || Platform.OS === "web") return;
  try {
    const originalFetch = globalThis.fetch.bind(globalThis);
    const { fetch: nitroFetch } = require("react-native-nitro-fetch");
    if (typeof nitroFetch === "function") {
      const wrappedFetch = (input: unknown, init?: unknown) => {
        if (getFetchUrl(input).startsWith("http://")) {
          return (originalFetch as any)(input, init);
        }
        return (nitroFetch as any)(input, init);
      };
      (globalThis as any).fetch = wrappedFetch;
      initialized = true;
    }
  } catch {
    // Falls back gracefully to default React Native fetch in environments without native Nitro bindings
  }
}
