import { createMMKV, type MMKV } from "react-native-mmkv";
import type { StateStorage } from "zustand/middleware";

export const appStorage: MMKV = createMMKV({
  id: "console-mobile-storage",
});

/**
 * Synchronous MMKV storage adapter for Zustand persist middleware.
 * Eliminates bridge overhead with zero-latency C++ JSI memory-mapped access.
 */
export const mmkvZustandStorage: StateStorage = {
  getItem: (name: string): string | null => {
    const value = appStorage.getString(name);
    return value ?? null;
  },
  setItem: (name: string, value: string): void => {
    appStorage.set(name, value);
  },
  removeItem: (name: string): void => {
    appStorage.delete(name);
  },
};
