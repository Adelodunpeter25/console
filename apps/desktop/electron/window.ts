import { BrowserWindow } from "electron";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { loadWindowState, trackWindowState } from "./window-state";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export const MAIN_DIST = path.join(__dirname, "..");
export const RENDERER_DIST = path.join(__dirname, "..", "dist");

export function createMainWindow(): BrowserWindow {
  const windowState = loadWindowState();

  const win = new BrowserWindow({
    x: windowState.x,
    y: windowState.y,
    width: windowState.width,
    height: windowState.height,
    minWidth: 800,
    minHeight: 600,
    titleBarStyle: "hiddenInset",
    trafficLightPosition: { x: 16, y: 12 },
    backgroundColor: "#000000",
    show: false,
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: false,
    },
  });

  trackWindowState(win);

  win.once("ready-to-show", () => {
    if (windowState.isMaximized) {
      win.maximize();
    }
    win.show();
  });

  if (process.env.VITE_DEV_SERVER_URL) {
    win.loadURL(process.env.VITE_DEV_SERVER_URL);
  } else {
    win.loadFile(path.join(RENDERER_DIST, "index.html"));
  }

  return win;
}
