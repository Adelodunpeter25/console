/**
 * Restart command - Restart the daemon
 */
import { stopDaemon } from "./stop.js";
import { startDaemon } from "./start.js";

export async function restartDaemon(options: { port: string; host: string }): Promise<void> {
  console.log("Restarting daemon...");

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
  });
}
