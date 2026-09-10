/**
 * Start command - Launch the daemon
 */
import { spawn } from "node:child_process";
import * as path from "node:path";
import { ensureConsoleDir, writePidFile, saveConfig, loadConfig, resolvePortHost, getDaemonStatus, loadEnvFile, getEnvFilePath } from "../daemon-manager.js";
import type { StartOptions } from "../types.js";

interface ServerLaunch {
  cmd: string;
  args: string[];
}

/**
 * Resolve how to launch the server:
 * 1. Compiled install: this `console` binary IS the server (multi-call).
 *    Re-executes itself detached with CONSOLE_SERVE=1.
 * 2. CONSOLE_SERVER_BIN env override (explicit path to a server binary/source)
 * 3. Dev fallback: run apps/server/index.ts via bun from the source tree
 */
function resolveServerLaunch(): ServerLaunch & { serveEnv?: boolean } {
  const execBase = path.basename(process.execPath);
  // Under the bun runtime (dev) execPath is bun/bun-debug/node — not our binary.
  const compiled = !["bun", "bun-debug", "node"].includes(execBase);

  const envBin = process.env.CONSOLE_SERVER_BIN;
  if (envBin && !compiled) return { cmd: envBin, args: [] };

  if (compiled) return { cmd: process.execPath, args: [], serveEnv: true };

  const cliCommandsDir = path.dirname(new URL(import.meta.url).pathname);
  const serverPath = path.resolve(cliCommandsDir, "..", "..", "server", "index.ts");
  return { cmd: "bun", args: [serverPath] };
}

export async function startDaemon(options: StartOptions): Promise<void> {
  const status = await getDaemonStatus();

  if (status.running) {
    console.log(`Daemon is already running (PID: ${status.pid})`);
    console.log(`Server: http://${status.host}:${status.port}`);
    console.log(`Uptime: ${status.uptime}`);
    process.exit(1);
  }

  // Resolve effective port/host: CLI flag > env > saved config > default.
  // Running `console start --port 9090` saves 9090; a later bare
  // `console start` reuses it instead of silently resetting to 3000.
  const saved = await loadConfig();
  const { port, host } = resolvePortHost({ port: options.port, host: options.host }, saved);
  const reusedPort = !options.port && !process.env.PORT;
  const reusedHost = !options.host && !process.env.HOST;
  const config = { port, host, logLevel: "info" };
  await saveConfig(config);
  await ensureConsoleDir();
  if (reusedPort || reusedHost) {
    console.log(`Using saved config from ~/.console/config.json (port ${port}, host ${host}).`);
  }

  // Provider keys (e.g. FIRECRAWL_API_KEY) live in ~/.console/env so every
  // daemon picks them up without shell exports. Explicit process env wins.
  const fileEnv = await loadEnvFile();
  const baseEnv = { ...fileEnv, ...process.env };
  const firecrawlStatus =
    process.env.FIRECRAWL_API_KEY ?? fileEnv.FIRECRAWL_API_KEY
      ? "Firecrawl: API key configured"
      : `Firecrawl: no API key (keyless mode) — run 'console env' to add one`;

  // Resolve how to launch the server (binary install or dev source tree)
  const launch = resolveServerLaunch();

  if (options.daemon) {
    // Start as background daemon
    console.log(`Starting console agent daemon...`);
    console.log(`Port: ${port}`);
    console.log(`Host: ${host}`);
    console.log(firecrawlStatus);

    const env = {
      ...baseEnv,
      PORT: port,
      HOST: host,
      CONSOLE_DAEMON: "true",
    };

    const child = spawn(launch.cmd, [...launch.args], {
      detached: true,
      stdio: "ignore",
      env: launch.serveEnv ? { ...env, CONSOLE_SERVE: "1" } : env,
    });

    child.unref();

    // Wait a moment and check if process is still running
    await new Promise((resolve) => setTimeout(resolve, 1000));

    if (child.pid && child.exitCode === null) {
      await writePidFile(child.pid);
      console.log(`Daemon started successfully (PID: ${child.pid})`);
      console.log(`Server: http://${host}:${port}`);
      console.log(`Logs: ~/.console/logs/daemon.log`);
      console.log(`Run 'console logs' to view logs`);
      console.log(`Run 'console stop' to stop the daemon`);
    } else {
      console.log(`❌ Failed to start daemon`);
      process.exit(1);
    }
  } else {
    // Run in foreground
    console.log(`Starting console agent in foreground...`);
    console.log(`Port: ${port}`);
    console.log(`Host: ${host}`);
    console.log(firecrawlStatus);
    console.log(`Press Ctrl+C to stop`);

    const env = {
      ...baseEnv,
      PORT: port,
      HOST: host,
      CONSOLE_DAEMON: "true",
    };

    const child = spawn(launch.cmd, launch.args, {
      stdio: "inherit",
      env: launch.serveEnv ? { ...env, CONSOLE_SERVE: "1" } : env,
    });
    await new Promise<number>((resolve) => child.on("exit", (code) => resolve(code ?? 0)));
    process.exit(0);
  }
}
