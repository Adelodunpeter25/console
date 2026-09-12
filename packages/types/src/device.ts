export type DevicePlatform = "ios" | "android";
export type DeviceState = "booted" | "shutdown" | "booting";

export interface DeviceDescriptor {
  id: string;
  name: string;
  platform: DevicePlatform;
  state: DeviceState;
  model?: string;
  osVersion?: string;
  isAvailable: boolean;
}

export interface DeviceDiagnostics {
  xcodeInstalled: boolean;
  xcodeVersion?: string;
  simctlAvailable: boolean;
  androidSdkFound: boolean;
  adbAvailable: boolean;
  emulatorAvailable: boolean;
  diskFreeBytes: number;
  hasEnoughDiskSpace: boolean;
  errors: string[];
}

export interface DeviceActionRequest {
  action: "tap" | "swipe" | "type" | "key" | "home" | "back" | "volume_up" | "volume_down" | "power";
  x?: number; // 0.0 .. 1.0 normalized
  y?: number; // 0.0 .. 1.0 normalized
  endX?: number; // for swipe
  endY?: number; // for swipe
  durationMs?: number;
  text?: string;
  key?: string;
}

export interface DeviceOpenAppRequest {
  app: string;
}
