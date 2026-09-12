import { exec } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs";
import * as path from "node:path";
import { createAgentDeviceClient } from "agent-device";
import type { DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics } from "@console/types";

const execAsync = promisify(exec);

export function findAndroidSdk(): string | undefined {
  const candidates = [
    process.env.ANDROID_HOME,
    process.env.ANDROID_SDK_ROOT,
    process.env.HOME ? path.join(process.env.HOME, "Library", "Android", "sdk") : undefined,
    process.env.HOME ? path.join(process.env.HOME, "Android", "Sdk") : undefined,
    process.env.LOCALAPPDATA ? path.join(process.env.LOCALAPPDATA, "Android", "Sdk") : undefined,
  ].filter((p): p is string => Boolean(p && fs.existsSync(p)));

  return candidates[0];
}

export function resolveAdbPath(): string {
  const sdk = findAndroidSdk();
  if (sdk) {
    const candidate = path.join(sdk, "platform-tools", process.platform === "win32" ? "adb.exe" : "adb");
    if (fs.existsSync(candidate)) return candidate;
  }
  return "adb";
}

export function resolveEmulatorPath(): string {
  const sdk = findAndroidSdk();
  if (sdk) {
    const candidate = path.join(sdk, "emulator", process.platform === "win32" ? "emulator.exe" : "emulator");
    if (fs.existsSync(candidate)) return candidate;
  }
  return "emulator";
}

// Ensure platform-tools & emulator directories are present in PATH for child tools
const androidSdkRoot = findAndroidSdk();
if (androidSdkRoot) {
  const platformTools = path.join(androidSdkRoot, "platform-tools");
  const emulatorDir = path.join(androidSdkRoot, "emulator");
  const currentPath = process.env.PATH ?? "";
  const toPrepend: string[] = [];
  if (fs.existsSync(platformTools) && !currentPath.includes(platformTools)) {
    toPrepend.push(platformTools);
  }
  if (fs.existsSync(emulatorDir) && !currentPath.includes(emulatorDir)) {
    toPrepend.push(emulatorDir);
  }
  if (toPrepend.length > 0) {
    process.env.PATH = `${toPrepend.join(":")}:${currentPath}`;
  }
}

export class DeviceService {
  private client = createAgentDeviceClient({ session: "console-device-service" });

  async getDiagnostics(): Promise<DeviceDiagnostics> {
    let simctlAvailable = false;
    let xcodeInstalled = false;
    let xcodeVersion: string | undefined = undefined;

    if (process.platform === "darwin") {
      try {
        const { stdout } = await execAsync("xcrun simctl help");
        if (stdout) simctlAvailable = true;
        const xcodeVer = await execAsync("xcodebuild -version");
        if (xcodeVer.stdout) {
          xcodeInstalled = true;
          xcodeVersion = xcodeVer.stdout.trim().split("\n")[0];
        }
      } catch {}
    }

    let adbAvailable = false;
    let emulatorAvailable = false;
    let androidSdkFound = false;

    const adbBin = resolveAdbPath();
    const emuBin = resolveEmulatorPath();

    try {
      const { stdout: adbOut } = await execAsync(`"${adbBin}" version`);
      if (adbOut) adbAvailable = true;
    } catch {}

    try {
      const { stdout: emuOut } = await execAsync(`"${emuBin}" -version`);
      if (emuOut) emulatorAvailable = true;
    } catch {}

    if (findAndroidSdk() || adbAvailable) {
      androidSdkFound = true;
    }

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

    if (!simctlAvailable && process.platform === "darwin") {
      errors.push("Xcode Command Line Tools / simctl not found. Run `xcode-select --install`.");
    }
    if (!adbAvailable) {
      errors.push("Android Platform Tools (`adb`) not found in PATH.");
    }
    if (!hasEnoughDiskSpace && diskFreeBytes > 0) {
      errors.push("Low disk space. Less than 5 GB available.");
    }

    return {
      xcodeInstalled,
      xcodeVersion,
      simctlAvailable,
      androidSdkFound,
      adbAvailable,
      emulatorAvailable,
      diskFreeBytes,
      hasEnoughDiskSpace,
      errors,
    };
  }

  async listDevices(): Promise<DeviceDescriptor[]> {
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
    try {
      await this.client.devices.boot({ device: id, platform } as any);
    } catch {
      // Fallback
      if (platform === "ios") {
        await execAsync(`xcrun simctl boot "${id}"`);
      } else {
        exec(`"${resolveEmulatorPath()}" -avd "${id}"`);
      }
    }
  }

  async shutdownDevice(id: string, platform: "ios" | "android"): Promise<void> {
    try {
      await this.client.devices.shutdown({ device: id, platform } as any);
    } catch {
      // Fallback
      if (platform === "ios") {
        await execAsync(`xcrun simctl shutdown "${id}"`);
      } else {
        await execAsync(`"${resolveAdbPath()}" -s "${id}" emu kill`);
      }
    }
  }

  async openApp(id: string, platform: "ios" | "android", app: string): Promise<void> {
    try {
      await this.client.apps.open({ device: id, platform, app } as any);
    } catch {
      // Fallback
      if (platform === "ios") {
        await execAsync(`xcrun simctl launch "${id}" "${app}"`);
      } else {
        await execAsync(`"${resolveAdbPath()}" -s "${id}" shell monkey -p "${app}" -c android.intent.category.LAUNCHER 1`);
      }
    }
  }

  async interact(id: string, platform: "ios" | "android", req: DeviceActionRequest): Promise<void> {
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
    } else if (req.action === "swipe" && req.x !== undefined && req.y !== undefined && req.endX !== undefined && req.endY !== undefined) {
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

    // Hardware buttons and fallback inputs
    if (platform === "ios") {
      if (req.action === "home") {
        await execAsync(`xcrun simctl io "${id}" sendkey home`);
      } else if (req.action === "type" && req.text) {
        await execAsync(`xcrun simctl io "${id}" keyboard send "${req.text.replace(/"/g, '\\"')}"`);
      } else if (req.action === "appearance" && req.appearance) {
        await execAsync(`xcrun simctl ui "${id}" appearance "${req.appearance}"`);
      }
    } else {
      const adb = resolveAdbPath();
      if (req.action === "tap" && req.x !== undefined && req.y !== undefined) {
        const pxX = Math.round(req.x * 1080);
        const pxY = Math.round(req.y * 2400);
        await execAsync(`"${adb}" -s "${id}" shell input tap ${pxX} ${pxY}`);
      } else if (req.action === "swipe" && req.x !== undefined && req.y !== undefined && req.endX !== undefined && req.endY !== undefined) {
        const x1 = Math.round(req.x * 1080);
        const y1 = Math.round(req.y * 2400);
        const x2 = Math.round(req.endX * 1080);
        const y2 = Math.round(req.endY * 2400);
        const dur = req.durationMs ?? 300;
        await execAsync(`"${adb}" -s "${id}" shell input swipe ${x1} ${y1} ${x2} ${y2} ${dur}`);
      } else if (req.action === "type" && req.text) {
        const escaped = req.text.replace(/ /g, "%s").replace(/"/g, '\\"');
        await execAsync(`"${adb}" -s "${id}" shell input text "${escaped}"`);
      } else if (req.action === "home") {
        await execAsync(`"${adb}" -s "${id}" shell input keyevent 3`);
      } else if (req.action === "back") {
        await execAsync(`"${adb}" -s "${id}" shell input keyevent 4`);
      } else if (req.action === "volume_up") {
        await execAsync(`"${adb}" -s "${id}" shell input keyevent 24`);
      } else if (req.action === "volume_down") {
        await execAsync(`"${adb}" -s "${id}" shell input keyevent 25`);
      } else if (req.action === "power") {
        await execAsync(`"${adb}" -s "${id}" shell input keyevent 26`);
      } else if (req.action === "appearance" && req.appearance) {
        const mode = req.appearance === "dark" ? "yes" : "no";
        await execAsync(`"${adb}" -s "${id}" shell cmd uimode night ${mode}`);
      }
    }
  }

  createVideoStream(id: string, platform: "ios" | "android", signal?: AbortSignal): ReadableStream<Uint8Array> {
    let proc: ReturnType<typeof Bun.spawn> | undefined;

    return new ReadableStream<Uint8Array>({
      start(controller) {
        try {
          if (platform === "ios") {
            proc = Bun.spawn(["xcrun", "simctl", "io", id, "recordVideo", "--codec=h264", "--force", "-"], {
              stdout: "pipe",
              stderr: "ignore",
            });
          } else {
            proc = Bun.spawn([resolveAdbPath(), "-s", id, "exec-out", "screenrecord", "--output-format=h264", "-"], {
              stdout: "pipe",
              stderr: "ignore",
            });
          }

          if (!proc || !proc.stdout) {
            controller.close();
            return;
          }

          const reader = (proc.stdout as ReadableStream<Uint8Array>).getReader();
          const pump = async () => {
            try {
              while (true) {
                if (signal?.aborted) break;
                const { done, value } = await reader.read();
                if (done) break;
                if (value && value.byteLength > 0) {
                  controller.enqueue(value);
                }
              }
            } catch {
            } finally {
              try { controller.close(); } catch {}
              try { proc?.kill(); } catch {}
            }
          };
          pump();

          signal?.addEventListener("abort", () => {
            try { proc?.kill(); } catch {}
          });
        } catch (err) {
          try { controller.error(err); } catch {}
        }
      },
      cancel() {
        try { proc?.kill(); } catch {}
      },
    });
  }

  async screenshot(id: string, platform: "ios" | "android"): Promise<Buffer> {
    try {
      const res = await this.client.capture.screenshot({ device: id, platform } as any);
      if (res && Buffer.isBuffer(res)) return res;
      if (res && typeof (res as { data?: string }).data === "string") {
        return Buffer.from((res as { data: string }).data, "base64");
      }
    } catch {}

    // Fallback
    const tmpFile = `/tmp/console_device_${Date.now()}_${id}.png`;
    try {
      if (platform === "ios") {
        await execAsync(`xcrun simctl io "${id}" screenshot "${tmpFile}"`);
      } else {
        await execAsync(`"${resolveAdbPath()}" -s "${id}" exec-out screencap -p > "${tmpFile}"`);
      }
      const data = await import("node:fs/promises").then((f) => f.readFile(tmpFile));
      await import("node:fs/promises").then((f) => f.unlink(tmpFile).catch(() => {}));
      return data;
    } catch (err) {
      await import("node:fs/promises").then((f) => f.unlink(tmpFile).catch(() => {}));
      throw err;
    }
  }
}

export const deviceService = new DeviceService();
