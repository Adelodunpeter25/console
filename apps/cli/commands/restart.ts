/**
 * Restart command - Restart the daemon
 */
import { stopDaemon } from "./stop.js";
import { startDaemon } from "./start.js";
import { getDaemonStatus } from "../daemon-manager.js";
import type { RestartOptions } from "../types.js";

export async function restartDaemon(options: RestartOptions): Promise<void> {
  console.log("Restarting daemon...");

  // Preserve the running daemon's storage mode unless --dev was passed.
  const before = await getDaemonStatus().catch(() => ({ running: false as const }));

  // Stop if running
  try {
    await stopDaemon();
  } catch {
    // Ignore if not running
    console.log("Daemon was not running");
  }

  // Start with same options
  await startDaemon({
    port: options.port,
    host: options.host,
    daemon: true,
    ...(options.dev !== undefined
      ? { dev: options.dev }
      : "mode" in before && before.mode === "dev"
        ? { dev: true }
        : {}),
  });
}
