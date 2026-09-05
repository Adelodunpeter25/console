/**
 * Upgrade command - Blind re-download of the latest `console` binary.
 *
 * Personal-project semantics: no version tags, no "already current" check.
 * Every push to main rebuilds the rolling `console-server` release, so
 * upgrade always fetches and swaps — running it twice is harmless.
 */
import * as fs from "node:fs/promises";
import * as os from "node:os";
import * as path from "node:path";
import { getDaemonStatus, killDaemon, loadConfig } from "../daemon-manager.js";
import { startDaemon } from "./start.js";

const REPO = "Adelodunpeter25/console";
const BASE_URL = `https://github.com/${REPO}/releases/download/console-server`;

function resolveTarget() {
  const osName = process.platform === "darwin" ? "macos" : process.platform === "linux" ? "linux" : null;
  if (!osName) throw new Error(`Unsupported OS: ${process.platform} (only Linux and macOS are supported)`);
  const arch = process.arch === "arm64" ? "arm64" : process.arch === "x64" ? "x64" : null;
  if (!arch) throw new Error(`Unsupported architecture: ${process.arch} (only x64 and arm64 are supported)`);

  const suffix = `${osName}-${arch}`;
  const libAsset = osName === "linux" ? `libfff_c-${suffix}.so` : `libfff_c-${suffix}.dylib`;
  const libFile = osName === "linux" ? "libfff_c.so" : "libfff_c.dylib";

  const execBase = path.basename(process.execPath);
  const compiled = !["bun", "bun-debug", "node"].includes(execBase);
  if (compiled) {
    const dir = path.dirname(process.execPath);
    return { suffix, binPath: process.execPath, libPath: path.join(dir, libFile), libAsset };
  }
  const prefix = process.env.CONSOLE_INSTALL_DIR ?? path.join(os.homedir(), ".local", "bin");
  return { suffix, binPath: path.join(prefix, "console"), libPath: path.join(prefix, libFile), libAsset };
}

async function downloadTo(url: string, dest: string, mode: number): Promise<void> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`Download failed (${res.status}): ${url}`);
  const buf = Buffer.from(await res.arrayBuffer());
  await fs.mkdir(path.dirname(dest), { recursive: true });
  const tmp = `${dest}.new-${process.pid}`;
  await fs.writeFile(tmp, buf, { mode });
  await fs.rename(tmp, dest);
}

export async function upgradeDaemon(): Promise<void> {
  const target = resolveTarget();
  const before = await getDaemonStatus();
  const wasRunning = before.running;
  const port = before.port ?? (await loadConfig()).port;
  const host = before.host ?? (await loadConfig()).host;

  console.log(`Upgrading Console (${target.suffix})...`);
  await downloadTo(`${BASE_URL}/console-${target.suffix}`, target.binPath, 0o755);
  await downloadTo(`${BASE_URL}/${target.libAsset}`, target.libPath, 0o644);
  console.log("Binary swapped.");

  if (wasRunning) {
    console.log("Restarting daemon on the new binary...");
    if (before.pid) {
      try {
        await killDaemon(before.pid);
      } catch (error) {
        console.log(`Could not stop old daemon: ${error}`);
      }
    }
    await startDaemon({ port, host, daemon: true });
  } else {
    console.log("Daemon was not running. Run 'console start' to launch the new binary.");
  }
}
