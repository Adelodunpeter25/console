/**
 * OpenCode sidecar — ensures the `opencode` CLI exists and a local
 * `opencode serve` instance is running for the Console opencode provider.
 *
 * Lifecycle is owned by `console start` / `console stop`:
 * - start: ensure CLI (install if missing) → adopt or spawn serve →
 *   persist { port, password, pid } to opencode-serve.json in the console dir.
 * - stop: kill the serve process only if Console started it (managed).
 *
 * A single serve process multiplexes unlimited sessions/directories
 * (instances resolve per-request via `x-opencode-directory`), so one sidecar
 * serves every Console chat. Auth is HTTP basic (`opencode` + the
 * server-generated password captured from serve stdout); the server only ever
 * binds 127.0.0.1.
 */
import { spawn, execFile } from "node:child_process";
import { promisify } from "node:util";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as os from "node:os";
import * as net from "node:net";
import { existsSync } from "node:fs";
import { getConsoleDir } from "./daemon-manager.js";

const execFileAsync = promisify(execFile);

export const OPENCODE_DEFAULT_PORT = 4096;
export const OPENCODE_INSTALL_URL = "https://opencode.ai/install";

export interface SidecarState {
  port: number;
  password: string;
  pid: number | null;
  /** True when Console spawned this process (safe to kill on stop). */
  managed: boolean;
}

function stateFilePath(): string {
  return path.join(getConsoleDir(), "opencode-serve.json");
}

async function readState(): Promise<SidecarState | null> {
  try {
    const raw = await fs.readFile(stateFilePath(), "utf-8");
    const parsed = JSON.parse(raw) as SidecarState;
    if (typeof parsed.port !== "number" || typeof parsed.password !== "string") return null;
    return parsed;
  } catch {
    return null;
  }
}

async function writeState(state: SidecarState): Promise<void> {
  await fs.mkdir(getConsoleDir(), { recursive: true });
  await fs.writeFile(stateFilePath(), JSON.stringify(state, null, 2), { mode: 0o600 });
}

async function removeState(): Promise<void> {
  try {
    await fs.unlink(stateFilePath());
  } catch {
    // missing — fine
  }
}

function basicAuth(password: string): string {
  return `Basic ${Buffer.from(`opencode:${password}`).toString("base64")}`;
}

async function isServeUp(port: number, password: string): Promise<boolean> {
  try {
    const res = await fetch(`http://127.0.0.1:${port}/api/info`, {
      headers: { Authorization: basicAuth(password) },
      signal: AbortSignal.timeout(3000),
    });
    return res.ok || res.status === 404;
  } catch {
    return false;
  }
}

async function isPortFree(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const srv = net.createServer();
    srv.once("error", () => resolve(false));
    srv.once("listening", () => srv.close(() => resolve(true)));
    srv.listen(port, "127.0.0.1");
  });
}

/**
 * Resolve the opencode binary: `which opencode`, then the default install
 * location (~/.opencode/bin/opencode). Returns null when not installed.
 */
export async function resolveOpencodeBin(): Promise<string | null> {
  if (process.env.OPENCODE_BIN && existsSync(process.env.OPENCODE_BIN)) {
    return process.env.OPENCODE_BIN;
  }
  try {
    const { stdout } = await execFileAsync("which", ["opencode"]);
    const found = stdout.trim().split("\n")[0]?.trim();
    if (found) return found;
  } catch {
    // not on PATH
  }
  const fallback = path.join(os.homedir(), ".opencode", "bin", "opencode");
  return existsSync(fallback) ? fallback : null;
}

export async function getOpencodeVersion(bin: string): Promise<string | null> {
  try {
    const { stdout } = await execFileAsync(bin, ["--version"]);
    return stdout.trim() || null;
  } catch {
    return null;
  }
}

/**
 * Ensure the opencode CLI exists, installing it via the official install
 * script when `which opencode` (and the default location) come up empty.
 * Throws with guidance when installation fails.
 */
export async function ensureOpencodeCli(): Promise<{ bin: string; version: string | null; installed: boolean }> {
  const existing = await resolveOpencodeBin();
  if (existing) {
    return { bin: existing, version: await getOpencodeVersion(existing), installed: false };
  }
  console.log("opencode CLI not found — installing via opencode.ai/install...");
  try {
    await execFileAsync("sh", ["-c", `curl -fsSL ${OPENCODE_INSTALL_URL} | sh`], { timeout: 180_000 });
  } catch (error) {
    throw new Error(
      `Automatic opencode install failed (${error}). Install manually: curl -fsSL ${OPENCODE_INSTALL_URL} | sh`,
    );
  }
  const bin = await resolveOpencodeBin();
  if (!bin) {
    throw new Error(
      `opencode installed but binary not found on PATH or ~/.opencode/bin. Set OPENCODE_BIN explicitly.`,
    );
  }
  return { bin, version: await getOpencodeVersion(bin), installed: true };
}

async function pickPort(): Promise<number> {
  const preferred = Number(process.env.OPENCODE_SERVE_PORT ?? OPENCODE_DEFAULT_PORT);
  for (let port = preferred; port < preferred + 32; port++) {
    if (await isPortFree(port)) return port;
  }
  throw new Error(`No free port for opencode serve (tried ${preferred}-${preferred + 31})`);
}

/**
 * Ensure a local `opencode serve` is reachable. Adopts a previous Console
 * sidecar when its state file still answers; otherwise spawns a new one and
 * persists the generated password. Never touches a foreign server occupying
 * the port (picks the next free port instead).
 */
export async function ensureOpencodeServe(): Promise<{ port: number; reused: boolean }> {
  const previous = await readState();
  if (previous && (await isServeUp(previous.port, previous.password))) {
    return { port: previous.port, reused: true };
  }

  const { bin, version, installed } = await ensureOpencodeCli();
  if (installed) console.log(`opencode CLI installed (${version ?? "unknown version"}).`);

  const port = await pickPort();
  const logPath = path.join(getConsoleDir(), "logs", "opencode-serve.log");
  await fs.mkdir(path.dirname(logPath), { recursive: true });
  const logFd = await fs.open(logPath, "a");

  const child = spawn(bin, ["serve", "--port", String(port), "--hostname", "127.0.0.1"], {
    detached: true,
    stdio: ["ignore", logFd.createWriteStream(), logFd.createWriteStream()],
  });
  child.unref();

  // The server prints `server password <secret>` on stdout; poll the log.
  const deadline = Date.now() + 20_000;
  let password: string | null = null;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 250));
    try {
      const log = await fs.readFile(logPath, "utf-8");
      const match = log.match(/server password (\S+)/);
      if (match?.[1]) {
        password = match[1];
        break;
      }
    } catch {
      // log not flushed yet
    }
    if (child.exitCode !== null) {
      throw new Error(`opencode serve exited early (code ${child.exitCode}) — see ${logPath}`);
    }
  }
  if (!password) throw new Error(`Timed out waiting for opencode serve password — see ${logPath}`);

  // Wait until the API answers with the captured password.
  const readyDeadline = Date.now() + 20_000;
  while (Date.now() < readyDeadline) {
    if (await isServeUp(port, password)) break;
    await new Promise((r) => setTimeout(r, 250));
  }
  if (!(await isServeUp(port, password))) {
    throw new Error(`opencode serve did not become ready on 127.0.0.1:${port} — see ${logPath}`);
  }

  await writeState({ port, password, pid: child.pid ?? null, managed: true });
  return { port, reused: false };
}

/**
 * Stop the sidecar, but only when Console manages it (spawned by
 * ensureOpencodeServe). Adopted/foreign servers are left alone.
 */
export async function stopOpencodeServe(): Promise<boolean> {
  const state = await readState();
  if (!state) return false;
  await removeState();
  if (!state.managed || !state.pid) return false;
  try {
    process.kill(state.pid, 0);
  } catch {
    return true; // already gone
  }
  try {
    process.kill(state.pid, "SIGTERM");
    for (let i = 0; i < 50; i++) {
      await new Promise((r) => setTimeout(r, 100));
      try {
        process.kill(state.pid, 0);
      } catch {
        return true;
      }
    }
    process.kill(state.pid, "SIGKILL");
  } catch {
    // died between checks
  }
  return true;
}
