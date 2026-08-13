import { BrowserWindow } from "electron";
import { registerDialogIpc } from "./dialogs";
import { registerShellIpc } from "./shell";
import { registerAuthIpc } from "./auth";

export function registerAllIpc(getMainWindow: () => BrowserWindow | null): void {
  registerDialogIpc(getMainWindow);
  registerShellIpc();
  registerAuthIpc(getMainWindow);
}
