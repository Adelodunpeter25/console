import { exec } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs";
import * as path from "node:path";
import type { DeviceActionRequest, DeviceDescriptor } from "@console/types";

const execAsync = promisify(exec);

export function resolveServeSimCmd(): string {
  const localBin = path.resolve(process.cwd(), "node_modules/.bin/serve-sim");
  if (fs.existsSync(localBin)) return `"${localBin}"`;
  const appsServerBin = path.resolve(process.cwd(), "apps/server/node_modules/.bin/serve-sim");
  if (fs.existsSync(appsServerBin)) return `"${appsServerBin}"`;
  return "serve-sim";
}

export class IosDeviceManager {
  async getDiagnostics(): Promise<{
    simctlAvailable: boolean;
    xcodeInstalled: boolean;
    xcodeVersion?: string;
  }> {
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

    return {
      simctlAvailable,
      xcodeInstalled,
      xcodeVersion,
    };
  }

  async listDevices(): Promise<DeviceDescriptor[]> {
    if (process.platform !== "darwin") return [];

    try {
      const { stdout } = await execAsync("xcrun simctl list devices available -j");
      const json = JSON.parse(stdout) as {
        devices?: Record<
          string,
          Array<{
            udid: string;
            name: string;
            state: string;
            isAvailable?: boolean;
            deviceTypeIdentifier?: string;
          }>
        >;
      };

      const results: DeviceDescriptor[] = [];
      for (const [runtime, list] of Object.entries(json.devices ?? {})) {
        const osName = runtime.replace(/^com\.apple\.CoreSimulator\.SimRuntime\./, "").replace(/-/g, " ");
        for (const d of list) {
          results.push({
            id: d.udid,
            name: d.name,
            platform: "ios",
            state: d.state.toLowerCase() === "booted" ? "booted" : "shutdown",
            model: d.name,
            osVersion: osName,
            isAvailable: d.isAvailable !== false,
          });
        }
      }
      return results;
    } catch {
      return [];
    }
  }

  async boot(id: string): Promise<void> {
    await execAsync(`xcrun simctl boot "${id}"`);
  }

  async shutdown(id: string): Promise<void> {
    await execAsync(`xcrun simctl shutdown "${id}"`);
  }

  async openApp(id: string, app: string): Promise<void> {
    await execAsync(`xcrun simctl launch "${id}" "${app}"`);
  }

  async interact(id: string, req: DeviceActionRequest): Promise<void> {
    const serveSim = resolveServeSimCmd();
    if (req.action === "tap" && req.x !== undefined && req.y !== undefined) {
      try {
        await execAsync(`${serveSim} tap -d "${id}" ${req.x} ${req.y}`);
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
        const payload = JSON.stringify({
          type: "swipe",
          x: req.x,
          y: req.y,
          endX: req.endX,
          endY: req.endY,
        });
        await execAsync(`${serveSim} gesture -d "${id}" '${payload}'`);
        return;
      } catch {}
    } else if (req.action === "type" && req.text) {
      try {
        await execAsync(`${serveSim} type -d "${id}" "${req.text.replace(/"/g, '\\"')}"`);
        return;
      } catch {}
    } else if (req.action === "home") {
      try {
        await execAsync(`${serveSim} button home -d "${id}"`);
        return;
      } catch {}
    } else if (req.action === "lock" || req.action === "power") {
      try {
        await execAsync(`${serveSim} button lock -d "${id}"`);
        return;
      } catch {}
    } else if (req.action === "volume_up") {
      try {
        await execAsync(`${serveSim} button volume_up -d "${id}"`);
        return;
      } catch {}
    } else if (req.action === "volume_down") {
      try {
        await execAsync(`${serveSim} button volume_down -d "${id}"`);
        return;
      } catch {}
    }

    // Fallbacks
    if (req.action === "home") {
      await execAsync(`xcrun simctl io "${id}" sendkey home`);
    } else if (req.action === "type" && req.text) {
      await execAsync(`xcrun simctl io "${id}" keyboard send "${req.text.replace(/"/g, '\\"')}"`);
    } else if (req.action === "appearance" && req.appearance) {
      await execAsync(`xcrun simctl ui "${id}" appearance "${req.appearance}"`);
    }
  }

  async captureStreamFrame(id: string): Promise<{ data: Buffer; mimeType: string }> {
    const proc = Bun.spawn(["xcrun", "simctl", "io", id, "screenshot", "--type=jpeg", "-"], {
      stdout: "pipe",
      stderr: "ignore",
    });
    const data = Buffer.from(await new Response(proc.stdout).arrayBuffer());
    return { data, mimeType: "image/jpeg" };
  }

  async screenshot(id: string): Promise<Buffer> {
    const proc = Bun.spawn(["xcrun", "simctl", "io", id, "screenshot", "--type=png", "-"], {
      stdout: "pipe",
      stderr: "ignore",
    });
    return Buffer.from(await new Response(proc.stdout).arrayBuffer());
  }
}

export const iosDeviceManager = new IosDeviceManager();
