/**
 * Stop command - Terminate the daemon
 */
import { getDaemonStatus, killDaemon } from "../daemon-manager.js";
import { stopOpencodeServe } from "../opencode-sidecar.js";

export async function stopDaemon(): Promise<void> {
  const status = await getDaemonStatus();

  if (!status.running) {
    console.log("Daemon is not running");
    process.exit(1);
  }

  console.log(`Stopping daemon (PID: ${status.pid})...`);

  try {
    await killDaemon(status.pid!);
    console.log("Daemon stopped successfully");
  } catch (error) {
    console.log(`Failed to stop daemon: ${error}`);
    process.exit(1);
  }

  try {
    if (await stopOpencodeServe()) console.log("OpenCode serve sidecar stopped");
  } catch (error) {
    console.log(`Failed to stop OpenCode serve sidecar: ${error}`);
  }
}
