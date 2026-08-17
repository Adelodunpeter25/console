import { BrowserWindow, screen, type Rectangle } from "electron";
import { loadStoredWindowState, saveStoredWindowState, type StoredWindowState } from "./storage";

const DEFAULT_STATE: StoredWindowState = {
  width: 1280,
  height: 800,
  isMaximized: false,
};

function isInsideAnyDisplay(bounds: Partial<Rectangle>): boolean {
  if (bounds.x === undefined || bounds.y === undefined) return false;
  const displays = screen.getAllDisplays();
  return displays.some((display) => {
    const { x, y, width, height } = display.bounds;
    return (
      (bounds.x ?? 0) >= x - 20 &&
      (bounds.y ?? 0) >= y - 20 &&
      (bounds.x ?? 0) < x + width &&
      (bounds.y ?? 0) < y + height
    );
  });
}

export function loadWindowState(): StoredWindowState {
  try {
    const data = loadStoredWindowState();
    if (data && typeof data.width === "number" && typeof data.height === "number") {
      if (data.x !== undefined && data.y !== undefined && isInsideAnyDisplay(data)) {
        return data;
      }
      return { width: data.width, height: data.height, isMaximized: data.isMaximized };
    }
  } catch {}
  return DEFAULT_STATE;
}

export function trackWindowState(win: BrowserWindow): void {
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  const save = () => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      try {
        if (win.isDestroyed()) return;
        const isMaximized = win.isMaximized();
        let state: StoredWindowState;

        if (isMaximized) {
          const prevState = loadWindowState();
          state = { ...prevState, isMaximized: true };
        } else {
          const bounds = win.getBounds();
          state = {
            x: bounds.x,
            y: bounds.y,
            width: bounds.width,
            height: bounds.height,
            isMaximized: false,
          };
        }

        saveStoredWindowState(state);
      } catch {}
    }, 200);
  };

  win.on("resize", save);
  win.on("move", save);
  win.on("close", save);
}
