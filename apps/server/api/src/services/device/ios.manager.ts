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
        if (fs.existsSync("/Applications/Xcode.app")) {
          xcodeInstalled = true;
          try {
            const versionPlist = fs.readFileSync("/Applications/Xcode.app/Contents/version.plist", "utf-8");
            const match = versionPlist.match(/<key>CFBundleShortVersionString<\/key>\s*<string>([^<]+)<\/string>/);
            if (match) {
              xcodeVersion = `Xcode ${match[1]}`;
            }
          } catch {}
        }
        const { stdout } = await execAsync("xcrun simctl help");
        if (stdout) simctlAvailable = true;
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
    const serveSim = resolveServeSimCmd();
    try {
      await execAsync(`${serveSim} --kill "${id}"`);
    } catch {}
    await execAsync(`xcrun simctl shutdown "${id}"`);
  }

  async shutdownAll(): Promise<void> {
    const serveSim = resolveServeSimCmd();
    try {
      await execAsync(`${serveSim} --kill`);
    } catch {}
    try {
      await execAsync("xcrun simctl shutdown all");
    } catch {}
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

  private activeStreams = new Map<string, { streamUrl: string }>();

  async ensureServeSim(id: string): Promise<string | undefined> {
    const existing = this.activeStreams.get(id);
    if (existing) {
      try {
        const probe = await fetch(existing.streamUrl, { method: "HEAD", signal: AbortSignal.timeout(1000) });
        if (probe.ok) return existing.streamUrl;
      } catch {}
    }

    try {
      const serveSim = resolveServeSimCmd();
      const { stdout } = await execAsync(`${serveSim} --detach -q "${id}"`);
      const info = JSON.parse(stdout.trim()) as { streamUrl?: string };
      if (info.streamUrl) {
        this.activeStreams.set(id, { streamUrl: info.streamUrl });
        return info.streamUrl;
      }
    } catch {}

    return undefined;
  }

  async createIosStream(id: string, signal?: AbortSignal): Promise<ReadableStream<Uint8Array>> {
    const streamUrl = await this.ensureServeSim(id);
    if (streamUrl) {
      try {
        const res = await fetch(streamUrl, { signal });
        if (res.ok && res.body) {
          return res.body as ReadableStream<Uint8Array>;
        }
      } catch {}
    }

    // Fallback: fast continuous in-memory streaming
    return new ReadableStream<Uint8Array>({
      async start(controller) {
        const boundary = "frame";
        const encoder = new TextEncoder();
        while (!signal?.aborted) {
          try {
            const proc = Bun.spawn(["xcrun", "simctl", "io", id, "screenshot", "--type=jpeg", "-"], {
              stdout: "pipe",
              stderr: "ignore",
            });
            const data = new Uint8Array(await new Response(proc.stdout).arrayBuffer());
            if (signal?.aborted) break;
            if (data.length > 0) {
              const header = `--${boundary}\r\nContent-Type: image/jpeg\r\nContent-Length: ${data.length}\r\n\r\n`;
              controller.enqueue(encoder.encode(header));
              controller.enqueue(data);
              controller.enqueue(encoder.encode("\r\n"));
            }
          } catch {
            break;
          }
        }
        try { controller.close(); } catch {}
      },
    });
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
