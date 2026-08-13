import { dialog, ipcMain, BrowserWindow } from "electron";
import fs from "node:fs/promises";
import path from "node:path";
import { getMimeType } from "../utils/mime";
import type { PickedImageResult } from "../types";

export function registerDialogIpc(getMainWindow: () => BrowserWindow | null): void {
  ipcMain.handle("dialog:confirm", async (_event, { title, message }: { title: string; message: string }) => {
    const win = getMainWindow();
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
    const win = getMainWindow();
    if (!win) return null;
    const result = await dialog.showOpenDialog(win, {
      properties: ["openDirectory"],
    });
    if (result.canceled || result.filePaths.length === 0) return null;
    return result.filePaths[0];
  });

  ipcMain.handle("dialog:pickImages", async () => {
    const win = getMainWindow();
    if (!win) return [];
    const result = await dialog.showOpenDialog(win, {
      properties: ["openFile", "multiSelections"],
      filters: [{ name: "Images", extensions: ["png", "jpg", "jpeg", "webp", "gif"] }],
    });
    if (result.canceled || result.filePaths.length === 0) return [];

    const images: PickedImageResult[] = await Promise.all(
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
    return images.filter((img): img is PickedImageResult => img !== null);
  });
}
