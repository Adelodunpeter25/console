import { app, ipcMain, shell, Notification } from "electron";

export function registerShellIpc(): void {
  ipcMain.handle("shell:openExternal", async (_event, { url }: { url: string }) => {
    await shell.openExternal(url);
  });

  ipcMain.handle("notification:show", async (_event, { title, body }: { title: string; body: string }) => {
    new Notification({ title, body }).show();
  });

  ipcMain.handle("app:getVersion", () => app.getVersion());
}
