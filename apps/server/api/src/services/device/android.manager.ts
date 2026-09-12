import { exec } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs";
import * as path from "node:path";
import type { DeviceActionRequest, DeviceDescriptor } from "@console/types";

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

// Automatically append platform-tools and emulator to PATH if available
const sdkRoot = findAndroidSdk();
if (sdkRoot) {
  const platformTools = path.join(sdkRoot, "platform-tools");
  const emulatorDir = path.join(sdkRoot, "emulator");
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

export class AndroidDeviceManager {
  async isAdbAvailable(): Promise<boolean> {
    try {
      const adb = resolveAdbPath();
      const { stdout } = await execAsync(`"${adb}" version`);
      return Boolean(stdout);
    } catch {
      return false;
    }
  }

  async isEmulatorAvailable(): Promise<boolean> {
    try {
      const emu = resolveEmulatorPath();
      const { stdout } = await execAsync(`"${emu}" -version`);
      return Boolean(stdout);
    } catch {
      return false;
    }
  }

  async listDevices(): Promise<DeviceDescriptor[]> {
    const adb = resolveAdbPath();
    const emu = resolveEmulatorPath();
    const devices: DeviceDescriptor[] = [];
    const runningSerialMap = new Map<string, string>(); // avdName -> serial

    try {
      const { stdout: adbOut } = await execAsync(`"${adb}" devices -l`);
      const lines = adbOut.trim().split("\n").slice(1);
      for (const line of lines) {
        const parts = line.trim().split(/\s+/);
        if (parts.length >= 2 && parts[1] === "device") {
          const serial = parts[0]!;
          let name = serial;
          try {
            const { stdout: modelOut } = await execAsync(`"${adb}" -s "${serial}" shell getprop ro.product.model`);
            if (modelOut.trim()) name = modelOut.trim();
          } catch {}

          try {
            const { stdout: avdOut } = await execAsync(`"${adb}" -s "${serial}" emu avd name`);
            const avdName = avdOut.trim().split("\n")[0]?.trim();
            if (avdName) runningSerialMap.set(avdName, serial);
          } catch {}

          devices.push({
            id: serial,
            name: `${name} (${serial})`,
            platform: "android",
            state: "booted",
            model: name,
            isAvailable: true,
          });
        }
      }
    } catch {}

    // Also list configured AVDs that might not be running yet
    try {
      const { stdout: avdListOut } = await execAsync(`"${emu}" -list-avds`);
      const avds = avdListOut.trim().split("\n").filter(Boolean);
      for (const avd of avds) {
        const avdName = avd.trim();
        if (!runningSerialMap.has(avdName) && !devices.some((d) => d.id === avdName)) {
          devices.push({
            id: avdName,
            name: `${avdName} (Android Emulator)`,
            platform: "android",
            state: "shutdown",
            model: avdName,
            isAvailable: true,
          });
        }
      }
    } catch {}

    return devices;
  }

  async boot(id: string): Promise<void> {
    const emu = resolveEmulatorPath();
    exec(`"${emu}" -avd "${id}"`);
  }

  async shutdown(id: string): Promise<void> {
    const adb = resolveAdbPath();
    await execAsync(`"${adb}" -s "${id}" emu kill`);
  }

  async openApp(id: string, app: string): Promise<void> {
    const adb = resolveAdbPath();
    await execAsync(`"${adb}" -s "${id}" shell monkey -p "${app}" -c android.intent.category.LAUNCHER 1`);
  }

  async interact(id: string, req: DeviceActionRequest): Promise<void> {
    const adb = resolveAdbPath();
    if (req.action === "tap" && req.x !== undefined && req.y !== undefined) {
      const pxX = Math.round(req.x * 1080);
      const pxY = Math.round(req.y * 2400);
      await execAsync(`"${adb}" -s "${id}" shell input tap ${pxX} ${pxY}`);
    } else if (
      req.action === "swipe" &&
      req.x !== undefined &&
      req.y !== undefined &&
      req.endX !== undefined &&
      req.endY !== undefined
    ) {
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

  createH264Stream(id: string, signal?: AbortSignal): ReadableStream<Uint8Array> {
    const adb = resolveAdbPath();
    let proc: ReturnType<typeof Bun.spawn> | undefined;

    return new ReadableStream<Uint8Array>({
      start(controller) {
        try {
          proc = Bun.spawn(
            [
              adb,
              "-s",
              id,
              "exec-out",
              "screenrecord",
              "--output-format=h264",
              "--size",
              "720x1280",
              "--bit-rate",
              "4000000",
              "-",
            ],
            {
              stdout: "pipe",
              stderr: "ignore",
            },
          );

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

  async captureStreamFrame(id: string): Promise<{ data: Buffer; mimeType: string }> {
    const adb = resolveAdbPath();
    const proc = Bun.spawn([adb, "-s", id, "exec-out", "screencap", "-p"], {
      stdout: "pipe",
      stderr: "ignore",
    });
    const data = Buffer.from(await new Response(proc.stdout).arrayBuffer());
    return { data, mimeType: "image/png" };
  }

  async screenshot(id: string): Promise<Buffer> {
    const adb = resolveAdbPath();
    const proc = Bun.spawn([adb, "-s", id, "exec-out", "screencap", "-p"], {
      stdout: "pipe",
      stderr: "ignore",
    });
    return Buffer.from(await new Response(proc.stdout).arrayBuffer());
  }
}

export const androidDeviceManager = new AndroidDeviceManager();
