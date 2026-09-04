/**
 * Server Entry Point — Starts the Hono API on Bun.serve (0.0.0.0:3000).
 * Terminal WebSocket upgrades ride the same server natively.
 * Supports daemon mode with logging and graceful shutdown.
 */
import "./agent/src/tools/fff-bootstrap.js";
import { createApiApp } from "./api/src/app.js";
import {
  isTerminalUpgradeRequest,
  terminalWebsocketHandlers,
  type TerminalSocketData,
} from "./api/src/terminal/socket.route.js";
import { terminalPtyManager } from "./api/src/terminal/pty.manager.js";
import * as fs from "node:fs/promises";
import * as path from "node:path";

import { getConsoleStorageDir } from "./agent/src/session/apppaths.js";

const app = createApiApp();
const port = Number.parseInt(process.env.PORT || "3000", 10);
const host = process.env.HOST || "0.0.0.0";
const isDaemon = process.env.CONSOLE_DAEMON === "true";

// Logging setup
let logFile: string | null = null;
let logStream: fs.FileHandle | null = null;

async function setupLogging(): Promise<void> {
  if (!isDaemon) return;

  const consoleDir = getConsoleStorageDir();
  const logsDir = path.join(consoleDir, "logs");

  try {
    await fs.mkdir(logsDir, { recursive: true });
    logFile = path.join(logsDir, "daemon.log");
    logStream = await fs.open(logFile, "a");
  } catch (error) {
    console.error(`Failed to setup logging: ${error}`);
  }
}

function log(message: string): void {
  const timestamp = new Date().toISOString();
  const logMessage = `[${timestamp}] ${message}\n`;

  // Always log to stdout/stderr
  if (isDaemon) {
    process.stdout.write(logMessage);
  } else {
    console.log(message);
  }

  // Write to log file in daemon mode
  if (logStream) {
    logStream.write(logMessage);
  }
}

async function shutdown(): Promise<void> {
  log("Shutting down server...");

  // Kill every tracked PTY so shells don't leak after the server exits.
  terminalPtyManager.killAll();

  // Close log stream
  if (logStream) {
    await logStream.close();
  }

  process.exit(0);
}

// Graceful shutdown handlers
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);

// Start server
async function startServer(): Promise<void> {
  await setupLogging();

  Bun.serve<TerminalSocketData>({
    port,
    hostname: host,
    // SSE agent-run streams can sit silent for minutes while a tool executes.
    // Bun's default idle timeout (10s) kills such connections mid-run — the
    // client then sees "SSE stream error" while the server keeps running the
    // agent (sidebar stays "Working"). 0 disables the timeout, matching the
    // old node:http bridging behavior.
    idleTimeout: 0,
    fetch(req, server) {
      if (isTerminalUpgradeRequest(req)) {
        // Hijack the socket; handlers take over once the upgrade completes.
        const upgraded = server.upgrade(req, { data: { url: req.url, sessionId: null, paused: false } });
        if (upgraded) return undefined;
        return new Response("Terminal WebSocket upgrade failed", { status: 400 });
      }
      return app.fetch(req);
    },
    websocket: terminalWebsocketHandlers.websocket,
  });

  log(`Console Agent Server running on http://${host}:${port}`);
  log(`API Base: http://${host}:${port}/api (Accepting connections from all hosts/devices)`);
  log(`Mode: ${isDaemon ? "daemon" : "foreground"}`);

  if (isDaemon) {
    log(`Logs: ${logFile}`);
  }
}

startServer().catch((error) => {
  console.error("Failed to start server:", error);
  process.exit(1);
});
