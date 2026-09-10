/**
 * Status command - Check daemon status
 */
import { getDaemonStatus } from "../daemon-manager.js";

export async function statusDaemon(): Promise<void> {
  const status = await getDaemonStatus();

  if (!status.running) {
    console.log("Daemon is not running");
    console.log("Run 'console start' to start the daemon");
    process.exit(0);
  }

  console.log("Daemon is running");
  console.log(`PID: ${status.pid}`);
  console.log(`Uptime: ${status.uptime || "unknown"}`);
  console.log(`Server: http://${status.host}:${status.port}`);
}
