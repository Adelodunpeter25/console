/**
 * Daemon Manager - Handles PID file management and process control
 */
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as os from "node:os";
import { existsSync } from "node:fs";
import { exec } from "node:child_process";
import { promisify } from "node:util";
import type { DaemonStatus, DaemonConfig } from "./types.js";

const execAsync = promisify(exec);

// Directory structure
const isDev = process.env.NODE_ENV === "development" || process.env.CONSOLE_ENV === "dev";
const homeDir = os.homedir();
const folderName = isDev ? ".console-dev" : ".console";
export const CONSOLE_DIR = path.join(homeDir, folderName);
const PID_FILE = path.join(CONSOLE_DIR, "daemon.pid");
export const LOGS_DIR = path.join(CONSOLE_DIR, "logs");
const CONFIG_FILE = path.join(CONSOLE_DIR, "config.json");

/**
 * Ensure console directory structure exists
 */
export async function ensureConsoleDir(): Promise<void> {
  try {
    await fs.mkdir(CONSOLE_DIR, { recursive: true });
    await fs.mkdir(LOGS_DIR, { recursive: true });
  } catch (error) {
    throw new Error(`Failed to create console directory: ${error}`);
  }
}

/**
 * Get the default daemon config
 */
export function getDefaultConfig(): DaemonConfig {
  return {
    port: process.env.PORT || "3000",
    host: process.env.HOST || "0.0.0.0",
    logLevel: process.env.LOG_LEVEL || "info",
  };
}

/**
 * Save daemon config
 */
export async function saveConfig(config: Partial<DaemonConfig>): Promise<void> {
  await ensureConsoleDir();
  const currentConfig = getDefaultConfig();
  const mergedConfig = { ...currentConfig, ...config };
  await fs.writeFile(CONFIG_FILE, JSON.stringify(mergedConfig, null, 2));
}

/**
 * Load daemon config
 */
export async function loadConfig(): Promise<DaemonConfig> {
  try {
    if (existsSync(CONFIG_FILE)) {
      const content = await fs.readFile(CONFIG_FILE, "utf-8");
      return JSON.parse(content) as DaemonConfig;
    }
  } catch (error) {
    console.warn(`Failed to load config, using defaults: ${error}`);
  }
  return getDefaultConfig();
}

/**
 * Write PID file
 */
export async function writePidFile(pid: number): Promise<void> {
  await ensureConsoleDir();
  await fs.writeFile(PID_FILE, pid.toString());
}

/**
 * Read PID file
 */
export async function readPidFile(): Promise<number | null> {
  try {
    if (existsSync(PID_FILE)) {
      const content = await fs.readFile(PID_FILE, "utf-8");
      return parseInt(content.trim(), 10);
    }
  } catch (error) {
    console.warn(`Failed to read PID file: ${error}`);
  }
  return null;
}

/**
 * Remove PID file
 */
export async function removePidFile(): Promise<void> {
  try {
    if (existsSync(PID_FILE)) {
      await fs.unlink(PID_FILE);
    }
  } catch (error) {
    console.warn(`Failed to remove PID file: ${error}`);
  }
}

/**
 * Check if a process is running by PID
 */
export async function isProcessRunning(pid: number): Promise<boolean> {
  try {
    // On Unix-like systems, use kill -0 to check if process exists
    process.kill(pid, 0);
    return true;
  } catch {
    // Process doesn't exist or we don't have permission
    return false;
  }
}

/**
 * Get daemon status
 */
export async function getDaemonStatus(): Promise<DaemonStatus> {
  const pid = await readPidFile();

  if (!pid) {
    return { running: false };
  }

  const running = await isProcessRunning(pid);

  if (!running) {
    // Clean up stale PID file
    await removePidFile();
    return { running: false };
  }

  // Get process info (uptime)
  let uptime: string | undefined;
  try {
    const { stdout } = await execAsync(`ps -p ${pid} -o etime=`).catch(() => ({ stdout: "" }));
    uptime = stdout.trim() || undefined;
  } catch {
    // Process might have died between checks
    return { running: false };
  }

  const config = await loadConfig();

  return {
    running: true,
    pid,
    uptime,
    port: config.port,
    host: config.host,
  };
}

/**
 * Kill daemon process
 */
export async function killDaemon(pid: number): Promise<void> {
  try {
    process.kill(pid, "SIGTERM");

    // Wait up to 5 seconds for graceful shutdown
    for (let i = 0; i < 50; i++) {
      await new Promise((resolve) => setTimeout(resolve, 100));
      if (!(await isProcessRunning(pid))) {
        await removePidFile();
        return;
      }
    }

    // Force kill if still running
    process.kill(pid, "SIGKILL");
    await removePidFile();
  } catch (error) {
    console.error(`Failed to kill daemon: ${error}`);
    throw error;
  }
}
