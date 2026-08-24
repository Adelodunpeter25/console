/**
 * Start command - Launch the daemon
 */
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import * as path from "node:path";
import { ensureConsoleDir, writePidFile, saveConfig, getDaemonStatus } from "../daemon-manager.js";
import type { StartOptions } from "../types.js";

interface ServerLaunch {
  cmd: string;
  args: string[];
}

/**
 * Resolve how to launch the server:
 * 1. CONSOLE_SERVER_BIN env override (explicit path to a server binary/source)
 * 2. A `console-server` binary installed next to this CLI (compiled installs:
 *    both binaries live in the same dir, e.g. ~/.local/bin)
 * 3. Dev fallback: run apps/server/index.ts via bun from the source tree
 */
function resolveServerLaunch(): ServerLaunch {
  const envBin = process.env.CONSOLE_SERVER_BIN;
  if (envBin) return { cmd: envBin, args: [] };

  const execDir = path.dirname(process.execPath);
  const candidate = path.join(execDir, "console-server");
  if (existsSync(candidate)) return { cmd: candidate, args: [] };

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

  // Save config
  const config = {
    port: options.port,
    host: options.host,
    logLevel: "info",
  };
  await saveConfig(config);
  await ensureConsoleDir();

  // Resolve how to launch the server (binary install or dev source tree)
  const launch = resolveServerLaunch();

  if (options.daemon) {
    // Start as background daemon
    console.log(`Starting console agent daemon...`);
    console.log(`Port: ${options.port}`);
    console.log(`Host: ${options.host}`);

    const env = {
      ...process.env,
      PORT: options.port,
      HOST: options.host,
      CONSOLE_DAEMON: "true",
    };

    const child = spawn(launch.cmd, [...launch.args], {
      detached: true,
      stdio: "ignore",
      env,
    });

    child.unref();

    // Wait a moment and check if process is still running
    await new Promise((resolve) => setTimeout(resolve, 1000));

    if (child.pid && child.exitCode === null) {
      await writePidFile(child.pid);
      console.log(`Daemon started successfully (PID: ${child.pid})`);
      console.log(`Server: http://${options.host}:${options.port}`);
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
    console.log(`Port: ${options.port}`);
    console.log(`Host: ${options.host}`);
    console.log(`Press Ctrl+C to stop`);

    const env = {
      ...process.env,
      PORT: options.port,
      HOST: options.host,
      CONSOLE_DAEMON: "true",
    };

    const child = spawn(launch.cmd, launch.args, { stdio: "inherit", env });
    await new Promise<number>((resolve) => child.on("exit", (code) => resolve(code ?? 0)));
    process.exit(0);
  }
}
