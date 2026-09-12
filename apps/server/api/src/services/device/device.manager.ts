import { exec } from "node:child_process";
import { promisify } from "node:util";
import { createAgentDeviceClient } from "agent-device";
import type { DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics } from "@console/types";
import { androidDeviceManager, findAndroidSdk } from "./android.manager.js";
import { iosDeviceManager } from "./ios.manager.js";

const execAsync = promisify(exec);

export class DeviceManager {
  private client = createAgentDeviceClient({ session: "console-device-service" });

  async getDiagnostics(): Promise<DeviceDiagnostics> {
    const iosDiag = await iosDeviceManager.getDiagnostics();
    const adbAvailable = await androidDeviceManager.isAdbAvailable();
    const emulatorAvailable = await androidDeviceManager.isEmulatorAvailable();
    const androidSdkFound = Boolean(findAndroidSdk() || adbAvailable);

    let diskFreeBytes = 0;
    try {
      const { stdout: dfOut } = await execAsync("df -k .");
      const lines = dfOut.trim().split("\n");
      if (lines.length > 1) {
        const parts = lines[1]!.replace(/\s+/g, " ").split(" ");
        if (parts.length >= 4) {
          diskFreeBytes = Number.parseInt(parts[3]!, 10) * 1024;
        }
      }
    } catch {}

    const hasEnoughDiskSpace = diskFreeBytes > 5 * 1024 * 1024 * 1024; // > 5GB
    const errors: string[] = [];

    if (!iosDiag.simctlAvailable && process.platform === "darwin") {
      errors.push("Xcode Command Line Tools / simctl not found. Run `xcode-select --install`.");
    }
    if (!adbAvailable) {
      errors.push("Android Platform Tools (`adb`) not found in PATH or Android SDK.");
    }
    if (!hasEnoughDiskSpace && diskFreeBytes > 0) {
      errors.push("Low disk space. Less than 5 GB available.");
    }

    return {
      xcodeInstalled: iosDiag.xcodeInstalled,
      xcodeVersion: iosDiag.xcodeVersion,
      simctlAvailable: iosDiag.simctlAvailable,
      androidSdkFound,
      adbAvailable,
      emulatorAvailable,
      diskFreeBytes,
      hasEnoughDiskSpace,
      errors,
    };
  }

  async listDevices(): Promise<DeviceDescriptor[]> {
    const [iosList, androidList] = await Promise.all([
      iosDeviceManager.listDevices(),
      androidDeviceManager.listDevices(),
    ]);

    // If local discovery produced devices, return them
    const combined = [...iosList, ...androidList];
    if (combined.length > 0) {
      return combined;
    }

    // Fallback to agent-device library
    try {
      const rawList = await this.client.devices.list();
      return rawList
        .filter((d) => d.platform === "ios" || d.platform === "android")
        .map((d) => ({
          id: d.id,
          name: d.name,
          platform: d.platform as "ios" | "android",
          state: d.booted ? "booted" : "shutdown",
          model: d.name,
          osVersion: (d as { appleOs?: string }).appleOs,
          isAvailable: true,
        }));
    } catch {
      return [];
    }
  }

  async bootDevice(id: string, platform: "ios" | "android"): Promise<void> {
    if (platform === "ios") {
      await iosDeviceManager.boot(id);
    } else {
      await androidDeviceManager.boot(id);
    }
  }

  async shutdownDevice(id: string, platform: "ios" | "android"): Promise<void> {
    if (platform === "ios") {
      await iosDeviceManager.shutdown(id);
    } else {
      await androidDeviceManager.shutdown(id);
    }
  }

  async openApp(id: string, platform: "ios" | "android", app: string): Promise<void> {
    if (platform === "ios") {
      await iosDeviceManager.openApp(id, app);
    } else {
      await androidDeviceManager.openApp(id, app);
    }
  }

  async interact(id: string, platform: "ios" | "android", req: DeviceActionRequest): Promise<void> {
    // Try high-level agent-device client first for click/type/swipe
    if (req.action === "tap" && req.x !== undefined && req.y !== undefined) {
      try {
        await this.client.interactions.click({
          device: id,
          platform,
          x: req.x,
          y: req.y,
        } as any);
        return;
      } catch {}
    } else if (req.action === "type" && req.text) {
      try {
        await this.client.interactions.type({
          device: id,
          platform,
          text: req.text,
        } as any);
        return;
      } catch {}
    } else if (
      req.action === "swipe" &&
      req.x !== undefined &&
      req.y !== undefined &&
      req.endX !== undefined &&
      req.endY !== undefined
    ) {
      try {
        await this.client.interactions.swipe({
          device: id,
          platform,
          startX: req.x,
          startY: req.y,
          endX: req.endX,
          endY: req.endY,
          durationMs: req.durationMs ?? 300,
        } as any);
        return;
      } catch {}
    } else if (req.action === "appearance" && req.appearance) {
      try {
        await this.client.settings.update({
          device: id,
          platform,
          appearance: req.appearance,
        } as any);
        return;
      } catch {}
    }

    // Platform-specific direct fallback
    if (platform === "ios") {
      await iosDeviceManager.interact(id, req);
    } else {
      await androidDeviceManager.interact(id, req);
    }
  }

  createH264Stream(id: string, signal?: AbortSignal): ReadableStream<Uint8Array> {
    return androidDeviceManager.createH264Stream(id, signal);
  }

  async createIosStream(id: string, signal?: AbortSignal): Promise<ReadableStream<Uint8Array>> {
    return iosDeviceManager.createIosStream(id, signal);
  }

  async captureStreamFrame(id: string, platform: "ios" | "android"): Promise<{ data: Buffer; mimeType: string }> {
    if (platform === "ios") {
      return iosDeviceManager.captureStreamFrame(id);
    } else {
      return androidDeviceManager.captureStreamFrame(id);
    }
  }

  async screenshot(id: string, platform: "ios" | "android"): Promise<Buffer> {
    try {
      const res = await this.client.capture.screenshot({ device: id, platform } as any);
      if (res && Buffer.isBuffer(res)) return res;
      if (res && typeof (res as { data?: string }).data === "string") {
        return Buffer.from((res as { data: string }).data, "base64");
      }
    } catch {}

    if (platform === "ios") {
      return iosDeviceManager.screenshot(id);
    } else {
      return androidDeviceManager.screenshot(id);
    }
  }
}

export const deviceManager = new DeviceManager();
