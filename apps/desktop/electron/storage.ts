import { app, ipcMain } from "electron";
import fs from "node:fs";
import path from "node:path";

export interface StoredWindowState {
  x?: number;
  y?: number;
  width: number;
  height: number;
  isMaximized?: boolean;
}

interface StorageDocument {
  version: 1;
  window?: StoredWindowState;
  layout?: unknown;
}

function getStorageFilePath(): string {
  return path.join(app.getPath("userData"), "storage", "state.json");
}

function readDocument(): StorageDocument {
  try {
    const document = JSON.parse(fs.readFileSync(getStorageFilePath(), "utf-8")) as StorageDocument;
    if (document && document.version === 1) return document;
  } catch {
    // Missing or corrupt state falls back to defaults.
  }
  return { version: 1 };
}

function writeDocument(document: StorageDocument): void {
  const filePath = getStorageFilePath();
  const directory = path.dirname(filePath);
  fs.mkdirSync(directory, { recursive: true });

  const tempPath = `${filePath}.${process.pid}.tmp`;
  fs.writeFileSync(tempPath, JSON.stringify(document, null, 2), "utf-8");
  fs.renameSync(tempPath, filePath);
}

export function loadStoredWindowState(): StoredWindowState | null {
  return readDocument().window ?? null;
}

export function saveStoredWindowState(window: StoredWindowState): void {
  writeDocument({ ...readDocument(), version: 1, window });
}

export function registerStorageIpc(): void {
  ipcMain.handle("storage:load-layout", () => readDocument().layout ?? null);
  ipcMain.handle("storage:save-layout", (_event, layout: unknown) => {
    writeDocument({ ...readDocument(), version: 1, layout });
    return true;
  });
  ipcMain.on("storage:save-layout-sync", (event, layout: unknown) => {
    try {
      writeDocument({ ...readDocument(), version: 1, layout });
      event.returnValue = true;
    } catch {
      event.returnValue = false;
    }
  });
}
