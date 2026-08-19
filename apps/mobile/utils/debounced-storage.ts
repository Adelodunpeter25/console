import type { StateStorage } from "zustand/middleware";
import AsyncStorage from "@react-native-async-storage/async-storage";

/**
 * AsyncStorage wrapper that coalesces rapid `setItem` calls into one native
 * bridge write after a quiet period. Use it as the `storage` for zustand
 * `persist` middleware when the persisted state changes frequently (e.g. on
 * every SSE token during streaming) — without debouncing, each token would
 * trigger a JSON.stringify + AsyncStorage.setItem round trip.
 *
 * Reads (`getItem`) and removes (`removeItem`) are immediate; only writes
 * are debounced. A pending write is always flushed before a remove so the
 * two never race and leave stale data on disk.
 */
export function debouncedAsyncStorage(delayMs: number): StateStorage {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let pending: { name: string; value: string } | null = null;

  function flushPending(): void {
    if (pending) {
      void AsyncStorage.setItem(pending.name, pending.value).catch(() => {});
      pending = null;
    }
  }

  return {
    getItem: (name) => AsyncStorage.getItem(name),
    setItem: (name, value) => {
      pending = { name, value };
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        flushPending();
        timer = null;
      }, delayMs);
    },
    removeItem: (name) => {
      if (timer) {
        clearTimeout(timer);
        timer = null;
        pending = null;
      }
      void AsyncStorage.removeItem(name).catch(() => {});
    },
  };
}
