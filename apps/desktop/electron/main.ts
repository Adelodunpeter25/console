import { app, BrowserWindow } from "electron";
import { createMainWindow } from "./window";
import { registerAllIpc } from "./ipc";

let mainWindow: BrowserWindow | null = null;

function getMainWindow(): BrowserWindow | null {
  return mainWindow;
}

// Register all IPC handlers
registerAllIpc(getMainWindow);

app.whenReady().then(() => {
  mainWindow = createMainWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      mainWindow = createMainWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
