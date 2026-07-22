/**
 * Logs command - Tail daemon logs
 */
import * as path from "node:path";
import { existsSync } from "node:fs";
import { spawn } from "node:child_process";
import { CONSOLE_DIR, LOGS_DIR } from "../daemon-manager.js";
import type { LogsOptions } from "../types.js";

export async function logsDaemon(options: LogsOptions): Promise<void> {
  const consoleDir = CONSOLE_DIR;
  const logsDir = LOGS_DIR;
  const logFile = path.join(logsDir, "daemon.log");
  
  if (!existsSync(logFile)) {
    console.log("No log file found");
    console.log("Daemon may not have been started yet");
    console.log("Run 'console start' to start the daemon");
    process.exit(1);
  }

  console.log(`Daemon logs (${logFile})`);
  console.log("Press Ctrl+C to exit\n");

  if (options.follow) {
    // Use tail -f for following logs
    const tail = spawn("tail", ["-f", logFile], {
      stdio: "inherit",
    });

    tail.on("error", (error) => {
      console.error(`Failed to tail logs: ${error}`);
      process.exit(1);
    });

    // Handle graceful shutdown
    process.on("SIGINT", () => {
      tail.kill();
      process.exit(0);
    });
  } else {
    // Show last N lines
    const tail = spawn("tail", ["-n", options.lines, logFile], {
      stdio: "inherit",
    });

    tail.on("error", (error) => {
      console.error(`Failed to read logs: ${error}`);
      process.exit(1);
    });

    await new Promise<void>((resolve) => {
      tail.on("close", () => resolve());
    });
  }
}