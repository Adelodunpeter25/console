export interface PickedImageResult {
  name: string;
  data: string;
  mimeType: string;
}

export interface ElectronApi {
  confirmDialog: (title: string, message: string) => Promise<boolean>;
  pickFolder: () => Promise<string | null>;
  pickImages: () => Promise<PickedImageResult[]>;
  readDroppedImages: (paths: string[]) => Promise<PickedImageResult[]>;
  openExternal: (url: string) => Promise<void>;
  showNotification: (title: string, body: string) => Promise<void>;
  getAppVersion: () => Promise<string>;
}
