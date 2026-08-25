import { createMMKV, type MMKV } from "react-native-mmkv";

export const appStorage: MMKV = createMMKV({
  id: "console-mobile-storage",
});

/** Synchronous string storage adapter (MMKV, zero-latency JSI access). */
export const mmkvStringStorage = {
  getItem: (name: string): string | null => {
    return appStorage.getString(name) ?? null;
  },
  setItem: (name: string, value: string): void => {
    appStorage.set(name, value);
  },
  removeItem: (name: string): void => {
    appStorage.remove(name);
  },
};
