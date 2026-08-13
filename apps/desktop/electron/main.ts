import { app, BrowserWindow, dialog, ipcMain, shell, Notification } from "electron";
import path from "node:path";
import fs from "node:fs/promises";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

process.env.APP_ROOT = path.join(__dirname, "..");

export const MAIN_DIST = path.join(process.env.APP_ROOT, "dist-electron");
export const RENDERER_DIST = path.join(process.env.APP_ROOT, "dist");

process.env.VITE_PUBLIC = process.env.VITE_DEV_SERVER_URL
  ? path.join(process.env.APP_ROOT, "public")
  : RENDERER_DIST;

let win: BrowserWindow | null = null;

function getMimeType(filePath: string): string {
  const ext = path.extname(filePath).toLowerCase();
  switch (ext) {
    case ".png":
      return "image/png";
    case ".jpg":
    case ".jpeg":
      return "image/jpeg";
    case ".webp":
      return "image/webp";
    case ".gif":
      return "image/gif";
    case ".svg":
      return "image/svg+xml";
    default:
      return "application/octet-stream";
  }
}

function createWindow() {
  win = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 800,
    minHeight: 600,
    titleBarStyle: "hiddenInset",
    trafficLightPosition: { x: 16, y: 16 },
    backgroundColor: "#000000",
    show: false,
    webPreferences: {
      preload: path.join(__dirname, "preload.mjs"),
      nodeIntegration: false,
      contextIsolation: true,
      sandbox: false,
    },
  });

  win.once("ready-to-show", () => {
    win?.show();
  });

  if (process.env.VITE_DEV_SERVER_URL) {
    win.loadURL(process.env.VITE_DEV_SERVER_URL);
  } else {
    win.loadFile(path.join(RENDERER_DIST, "index.html"));
  }
}

// IPC Handlers
ipcMain.handle("dialog:confirm", async (_event, { title, message }: { title: string; message: string }) => {
  if (!win) return false;
  const result = await dialog.showMessageBox(win, {
    type: "question",
    buttons: ["Cancel", "OK"],
    defaultId: 1,
    cancelId: 0,
    title,
    message: title,
    detail: message,
  });
  return result.response === 1;
});

ipcMain.handle("dialog:pickFolder", async () => {
  if (!win) return null;
  const result = await dialog.showOpenDialog(win, {
    properties: ["openDirectory"],
  });
  if (result.canceled || result.filePaths.length === 0) return null;
  return result.filePaths[0];
});

ipcMain.handle("dialog:pickImages", async () => {
  if (!win) return [];
  const result = await dialog.showOpenDialog(win, {
    properties: ["openFile", "multiSelections"],
    filters: [{ name: "Images", extensions: ["png", "jpg", "jpeg", "webp", "gif"] }],
  });
  if (result.canceled || result.filePaths.length === 0) return [];

  const images = await Promise.all(
    result.filePaths.map(async (fp) => {
      const buffer = await fs.readFile(fp);
      return {
        name: path.basename(fp),
        data: buffer.toString("base64"),
        mimeType: getMimeType(fp),
      };
    }),
  );
  return images;
});

ipcMain.handle("dialog:readDroppedImages", async (_event, { paths }: { paths: string[] }) => {
  const images = await Promise.all(
    paths.map(async (fp) => {
      try {
        const buffer = await fs.readFile(fp);
        return {
          name: path.basename(fp),
          data: buffer.toString("base64"),
          mimeType: getMimeType(fp),
        };
      } catch {
        return null;
      }
    }),
  );
  return images.filter((img): img is { name: string; data: string; mimeType: string } => img !== null);
});

ipcMain.handle("shell:openExternal", async (_event, { url }: { url: string }) => {
  await shell.openExternal(url);
});

ipcMain.handle("notification:show", async (_event, { title, body }: { title: string; body: string }) => {
  new Notification({ title, body }).show();
});

ipcMain.handle("app:getVersion", () => app.getVersion());

app.whenReady().then(() => {
  createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
