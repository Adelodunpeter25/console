import { Platform } from "react-native";

let initialized = false;

/**
 * Initializes Nitro Fetch (powered by Cronet on Android and URLSession on iOS)
 * as the global fetch implementation for accelerated native networking, HTTP/3,
 * connection pre-warming, and direct native JSI bindings without JS bridge overhead.
 */
export function initNitroFetch(): void {
  if (initialized || Platform.OS === "web") return;
  try {
    const { fetch: nitroFetch } = require("react-native-nitro-fetch");
    if (typeof nitroFetch === "function") {
      (globalThis as any).fetch = nitroFetch;
      initialized = true;
    }
  } catch {
    // Falls back gracefully to default React Native fetch in environments without native Nitro bindings
  }
}
