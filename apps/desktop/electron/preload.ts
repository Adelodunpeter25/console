import { contextBridge, ipcRenderer } from "electron";
import type { ElectronApi } from "./types";

const electronApi: ElectronApi = {
  confirmDialog: (title: string, message: string) =>
    ipcRenderer.invoke("dialog:confirm", { title, message }),
  pickFolder: () => ipcRenderer.invoke("dialog:pickFolder"),
  pickImages: () => ipcRenderer.invoke("dialog:pickImages"),
  readDroppedImages: (paths: string[]) =>
    ipcRenderer.invoke("dialog:readDroppedImages", { paths }),
  openExternal: (url: string) => ipcRenderer.invoke("shell:openExternal", { url }),
  showNotification: (title: string, body: string) =>
    ipcRenderer.invoke("notification:show", { title, body }),
  getAppVersion: () => ipcRenderer.invoke("app:getVersion"),
  authLoginWithBrowser: (opts) => ipcRenderer.invoke("auth:loginWithBrowser", opts),
};

contextBridge.exposeInMainWorld("electronApi", electronApi);
