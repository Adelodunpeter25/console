import { LazyStore } from "@tauri-apps/plugin-store";

/**
 * Shared persisted UI preferences (sidebar width) backed by tauri-plugin-store.
 * Window size is handled by tauri-plugin-window-state. Values survive restarts.
 * Keys are namespaced so the store can grow without collisions.
 */
export const uiStore = new LazyStore("ui-preferences.json", {
  // Only persist on explicit set(); avoid the plugin's debounced auto-write
  // racing explicit saves during rapid drags.
  autoSave: false,
});

export const UI_KEYS = {
  sidebarWidth: "sidebar.width",
  rightSidebarWidth: "sidebar.rightWidth",
  sidebarOpen: "sidebar.open",
  rightSidebarOpen: "sidebar.rightOpen",
} as const;

export async function getSidebarWidth(): Promise<number | null> {
  const v = await uiStore.get<number>(UI_KEYS.sidebarWidth);
  return typeof v === "number" ? v : null;
}

export async function setSidebarWidth(width: number): Promise<void> {
  await uiStore.set(UI_KEYS.sidebarWidth, width);
  await uiStore.save();
}

export async function getRightSidebarWidth(): Promise<number | null> {
  const v = await uiStore.get<number>(UI_KEYS.rightSidebarWidth);
  return typeof v === "number" ? v : null;
}

export async function setRightSidebarWidth(width: number): Promise<void> {
  await uiStore.set(UI_KEYS.rightSidebarWidth, width);
  await uiStore.save();
}

export async function getSidebarOpen(): Promise<boolean | null> {
  const v = await uiStore.get<boolean>(UI_KEYS.sidebarOpen);
  return typeof v === "boolean" ? v : null;
}

export async function setSidebarOpen(open: boolean): Promise<void> {
  await uiStore.set(UI_KEYS.sidebarOpen, open);
  await uiStore.save();
}

export async function getRightSidebarOpen(): Promise<boolean | null> {
  const v = await uiStore.get<boolean>(UI_KEYS.rightSidebarOpen);
  return typeof v === "boolean" ? v : null;
}

export async function setRightSidebarOpen(open: boolean): Promise<void> {
  await uiStore.set(UI_KEYS.rightSidebarOpen, open);
  await uiStore.save();
}
