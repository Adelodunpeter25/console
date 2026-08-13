/**
 * Shared persisted UI preferences (sidebar width) backed by localStorage in Electron.
 */

export const UI_KEYS = {
  sidebarWidth: "console.sidebar.width",
  rightSidebarWidth: "console.sidebar.rightWidth",
  sidebarOpen: "console.sidebar.open",
  rightSidebarOpen: "console.sidebar.rightOpen",
} as const;

export async function getSidebarWidth(): Promise<number | null> {
  const v = localStorage.getItem(UI_KEYS.sidebarWidth);
  return v ? Number(v) : null;
}

export async function setSidebarWidth(width: number): Promise<void> {
  localStorage.setItem(UI_KEYS.sidebarWidth, String(width));
}

export async function getRightSidebarWidth(): Promise<number | null> {
  const v = localStorage.getItem(UI_KEYS.rightSidebarWidth);
  return v ? Number(v) : null;
}

export async function setRightSidebarWidth(width: number): Promise<void> {
  localStorage.setItem(UI_KEYS.rightSidebarWidth, String(width));
}

export async function getSidebarOpen(): Promise<boolean | null> {
  const v = localStorage.getItem(UI_KEYS.sidebarOpen);
  return v !== null ? v === "true" : null;
}

export async function setSidebarOpen(open: boolean): Promise<void> {
  localStorage.setItem(UI_KEYS.sidebarOpen, String(open));
}

export async function getRightSidebarOpen(): Promise<boolean | null> {
  const v = localStorage.getItem(UI_KEYS.rightSidebarOpen);
  return v !== null ? v === "true" : null;
}

export async function setRightSidebarOpen(open: boolean): Promise<void> {
  localStorage.setItem(UI_KEYS.rightSidebarOpen, String(open));
}
