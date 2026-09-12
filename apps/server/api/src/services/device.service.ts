import { exec } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs/promises";
import type { DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics } from "@console/types";

const execAsync = promisify(exec);

export class DeviceService {
  async getDiagnostics(): Promise<DeviceDiagnostics> {
    let simctlAvailable = false;
    let xcodeInstalled = false;
    let xcodeVersion: string | undefined = undefined;

    try {
      const { stdout } = await execAsync("xcrun simctl help");
      if (stdout) simctlAvailable = true;
      const xcodeVer = await execAsync("xcodebuild -version");
      if (xcodeVer.stdout) {
        xcodeInstalled = true;
        xcodeVersion = xcodeVer.stdout.trim().split("\n")[0];
      }
    } catch {
      // Not on macOS or Xcode CLI tools not installed
    }

    let adbAvailable = false;
    let emulatorAvailable = false;
    let androidSdkFound = false;

    try {
      const { stdout: adbOut } = await execAsync("adb version");
      if (adbOut) adbAvailable = true;
    } catch {}

    try {
      const { stdout: emuOut } = await execAsync("emulator -version");
      if (emuOut) emulatorAvailable = true;
    } catch {}

    if (process.env.ANDROID_HOME || process.env.ANDROID_SDK_ROOT || adbAvailable) {
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
    const devices: DeviceDescriptor[] = [];

    // iOS Simulators
    if (process.platform === "darwin") {
      try {
        const { stdout } = await execAsync("xcrun simctl list devices --json");
        const parsed = JSON.parse(stdout) as { devices: Record<string, Array<{ udid: string; name: string; state: string; isAvailable: boolean }>> };
        for (const [runtime, list] of Object.entries(parsed.devices)) {
          const osMatch = runtime.match(/iOS[- ]?([0-9.]+)/i);
          const osVersion = osMatch ? `iOS ${osMatch[1]}` : runtime;

          for (const item of list) {
            if (!item.isAvailable) continue;
            devices.push({
              id: item.udid,
              name: item.name,
              platform: "ios",
              state: item.state.toLowerCase() === "booted" ? "booted" : "shutdown",
              osVersion,
              isAvailable: item.isAvailable,
            });
          }
        }
      } catch {}
    }

    // Android Devices & Emulators
    try {
      const { stdout: adbOut } = await execAsync("adb devices -l");
      const lines = adbOut.trim().split("\n").slice(1);
      const runningSerialIds = new Set<string>();

      for (const line of lines) {
        if (!line.trim()) continue;
        const [serial, state, ...rest] = line.trim().replace(/\s+/g, " ").split(" ");
        if (!serial) continue;
        runningSerialIds.add(serial);

        const modelPart = rest.find((p) => p.startsWith("model:"));
        const model = modelPart ? modelPart.replace("model:", "") : serial;

        devices.push({
          id: serial,
          name: model,
          platform: "android",
          state: state === "device" ? "booted" : "booting",
          model,
          isAvailable: true,
        });
      }

      // Android stopped AVDs
      try {
        const { stdout: avdOut } = await execAsync("emulator -list-avds");
        const avds = avdOut.trim().split("\n");
        for (const avd of avds) {
          const name = avd.trim();
          if (!name) continue;
          if ([...runningSerialIds].some((s) => s.includes(name))) continue;

          devices.push({
            id: name,
            name,
            platform: "android",
            state: "shutdown",
            isAvailable: true,
          });
        }
      } catch {}
    } catch {}

    return devices;
  }

  async bootDevice(id: string, platform: "ios" | "android"): Promise<void> {
    if (platform === "ios") {
      await execAsync(`xcrun simctl boot "${id}"`);
    } else {
      exec(`emulator -avd "${id}"`);
    }
  }

  async shutdownDevice(id: string, platform: "ios" | "android"): Promise<void> {
    if (platform === "ios") {
      await execAsync(`xcrun simctl shutdown "${id}"`);
    } else {
      await execAsync(`adb -s "${id}" emu kill`);
    }
  }

  async openApp(id: string, platform: "ios" | "android", app: string): Promise<void> {
    if (platform === "ios") {
      await execAsync(`xcrun simctl launch "${id}" "${app}"`);
    } else {
      await execAsync(`adb -s "${id}" shell monkey -p "${app}" -c android.intent.category.LAUNCHER 1`);
    }
  }

  async interact(id: string, platform: "ios" | "android", req: DeviceActionRequest): Promise<void> {
    if (platform === "ios") {
      if (req.action === "home") {
        await execAsync(`xcrun simctl io "${id}" sendkey home`);
      } else if (req.action === "type" && req.text) {
        await execAsync(`xcrun simctl io "${id}" keyboard send "${req.text.replace(/"/g, '\\"')}"`);
      }
    } else {
      if (req.action === "tap" && req.x !== undefined && req.y !== undefined) {
        // Assume standard 1080x2400 if screen metrics aren't queried
        const pxX = Math.round(req.x * 1080);
        const pxY = Math.round(req.y * 2400);
        await execAsync(`adb -s "${id}" shell input tap ${pxX} ${pxY}`);
      } else if (req.action === "swipe" && req.x !== undefined && req.y !== undefined && req.endX !== undefined && req.endY !== undefined) {
        const x1 = Math.round(req.x * 1080);
        const y1 = Math.round(req.y * 2400);
        const x2 = Math.round(req.endX * 1080);
        const y2 = Math.round(req.endY * 2400);
        const dur = req.durationMs ?? 300;
        await execAsync(`adb -s "${id}" shell input swipe ${x1} ${y1} ${x2} ${y2} ${dur}`);
      } else if (req.action === "type" && req.text) {
        const escaped = req.text.replace(/ /g, "%s").replace(/"/g, '\\"');
        await execAsync(`adb -s "${id}" shell input text "${escaped}"`);
      } else if (req.action === "home") {
        await execAsync(`adb -s "${id}" shell input keyevent 3`);
      } else if (req.action === "back") {
        await execAsync(`adb -s "${id}" shell input keyevent 4`);
      } else if (req.action === "volume_up") {
        await execAsync(`adb -s "${id}" shell input keyevent 24`);
      } else if (req.action === "volume_down") {
        await execAsync(`adb -s "${id}" shell input keyevent 25`);
      } else if (req.action === "power") {
        await execAsync(`adb -s "${id}" shell input keyevent 26`);
      }
    }
  }

  async screenshot(id: string, platform: "ios" | "android"): Promise<Buffer> {
    const tmpFile = `/tmp/console_device_${Date.now()}_${id}.png`;
    try {
      if (platform === "ios") {
        await execAsync(`xcrun simctl io "${id}" screenshot "${tmpFile}"`);
      } else {
        await execAsync(`adb -s "${id}" exec-out screencap -p > "${tmpFile}"`);
      }
      const data = await fs.readFile(tmpFile);
      await fs.unlink(tmpFile).catch(() => {});
      return data;
    } catch (err) {
      await fs.unlink(tmpFile).catch(() => {});
      throw err;
    }
  }
}

export const deviceService = new DeviceService();
