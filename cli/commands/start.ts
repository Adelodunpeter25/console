/**
 * Start command - Launch the daemon
 */
import { spawn } from "node:child_process";
import * as path from "node:path";
import {
  ensureConsoleDir,
  writePidFile,
  saveConfig,
  getDaemonStatus,
} from "../daemon-manager.js";
import type { StartOptions } from "../types.js";

export async function startDaemon(options: StartOptions): Promise<void> {
  const status = await getDaemonStatus();
  
  if (status.running) {
    console.log(`❌ Daemon is already running (PID: ${status.pid})`);
    console.log(`   Server: http://${status.host}:${status.port}`);
    console.log(`   Uptime: ${status.uptime}`);
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

  // Determine server path - works for both local and global installation
  const serverPath = path.resolve(process.cwd(), "server", "index.ts");
  
  if (options.daemon) {
    // Start as background daemon
    console.log(`🚀 Starting console agent daemon...`);
    console.log(`   Port: ${options.port}`);
    console.log(`   Host: ${options.host}`);
    
    const args = [serverPath];
    const env = {
      ...process.env,
      PORT: options.port,
      HOST: options.host,
      CONSOLE_DAEMON: "true",
    };

    const child = spawn("npx", ["tsx", ...args], {
      detached: true,
      stdio: "ignore",
      env,
    });

    child.unref();
    
    // Wait a moment and check if process is still running
    await new Promise(resolve => setTimeout(resolve, 1000));
    
    if (child.pid && child.exitCode === null) {
      await writePidFile(child.pid);
      console.log(`✅ Daemon started successfully (PID: ${child.pid})`);
      console.log(`   Server: http://${options.host}:${options.port}`);
      console.log(`   Logs: ~/.console/logs/daemon.log`);
      console.log(`   Run 'console logs' to view logs`);
      console.log(`   Run 'console stop' to stop the daemon`);
    } else {
      console.log(`❌ Failed to start daemon`);
      process.exit(1);
    }
  } else {
    // Run in foreground
    console.log(`🚀 Starting console agent in foreground...`);
    console.log(`   Port: ${options.port}`);
    console.log(`   Host: ${options.host}`);
    console.log(`   Press Ctrl+C to stop`);
    
    process.env.PORT = options.port;
    process.env.HOST = options.host;
    process.env.CONSOLE_DAEMON = "true";
    
    // Import and run the server directly
    await import(serverPath);
  }
}