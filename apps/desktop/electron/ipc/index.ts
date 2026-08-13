import { BrowserWindow } from "electron";
import { registerDialogIpc } from "./dialogs";
import { registerShellIpc } from "./shell";

export function registerAllIpc(getMainWindow: () => BrowserWindow | null): void {
  registerDialogIpc(getMainWindow);
  registerShellIpc();
}
