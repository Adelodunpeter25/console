import { contextBridge, ipcRenderer } from "electron";

export interface ElectronApi {
  confirmDialog: (title: string, message: string) => Promise<boolean>;
  pickFolder: () => Promise<string | null>;
  pickImages: () => Promise<Array<{ name: string; data: string; mimeType: string }>>;
  readDroppedImages: (paths: string[]) => Promise<Array<{ name: string; data: string; mimeType: string }>>;
  openExternal: (url: string) => Promise<void>;
  showNotification: (title: string, body: string) => Promise<void>;
  getAppVersion: () => Promise<string>;
}

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
};

contextBridge.exposeInMainWorld("electronApi", electronApi);
