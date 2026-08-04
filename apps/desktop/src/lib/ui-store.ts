import { LazyStore } from "@tauri-apps/plugin-store";

/**
 * Shared persisted UI preferences (sidebar width, window size) backed by
 * tauri-plugin-store. Values survive restarts. Keys are namespaced so the
 * store can grow without collisions.
 */
export const uiStore = new LazyStore("ui-preferences.json", {
  // Only persist on explicit set(); avoid the plugin's debounced auto-write
  // racing explicit saves during rapid drags.
  autoSave: false,
});

export const UI_KEYS = {
  sidebarWidth: "sidebar.width",
  windowSize: "window.size",
} as const;

export async function getSidebarWidth(): Promise<number | null> {
  const v = await uiStore.get<number>(UI_KEYS.sidebarWidth);
  return typeof v === "number" ? v : null;
}

export async function setSidebarWidth(width: number): Promise<void> {
  await uiStore.set(UI_KEYS.sidebarWidth, width);
  await uiStore.save();
}

export async function getWindowSize(): Promise<{ width: number; height: number } | null> {
  const v = await uiStore.get<{ width: number; height: number }>(UI_KEYS.windowSize);
  return v ?? null;
}

export async function setWindowSize(size: { width: number; height: number }): Promise<void> {
  await uiStore.set(UI_KEYS.windowSize, size);
  await uiStore.save();
}
