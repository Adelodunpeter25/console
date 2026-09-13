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
import {
  isPortTunnelUpgradeRequest,
  parseTunnelPort,
  portTunnelWebsocketHandlers,
  type PortTunnelSocketData,
} from "./api/src/services/port-tunnel.socket.js";
import { terminalPtyManager } from "./api/src/terminal/pty.manager.js";
import { portRegistry } from "./api/src/services/port-registry.service.js";
import { SessionService } from "./api/src/services/session.service.js";
import * as fs from "node:fs/promises";
import * as path from "node:path";

import { getConsoleStorageDir } from "./agent/src/session/apppaths.js";

// Deleted chats stay restorable for 7 days, then the backend purges them
// permanently. The sweep itself runs once a day so any purge lands within
// ~24h of the 7-day mark.
const DELETED_CHAT_SWEEP_INTERVAL_MS = 24 * 60 * 60 * 1000;
let deletedChatSweepTimer: ReturnType<typeof setInterval> | undefined;

function sweepExpiredDeletedChats(): void {
  try {
    const purged = new SessionService().purgeExpiredDeletedSessions();
    if (purged.length > 0) log(`Purged ${purged.length} deleted chat(s) older than 7 days.`);
  } catch (error) {
    console.error(`Deleted-chat sweep failed: ${error}`);
  }
}

function startDeletedChatSweep(): void {
  // One startup pass so restarts don't wait a full day to catch up.
  sweepExpiredDeletedChats();
  if (deletedChatSweepTimer) return;
  deletedChatSweepTimer = setInterval(sweepExpiredDeletedChats, DELETED_CHAT_SWEEP_INTERVAL_MS);
  (deletedChatSweepTimer as unknown as { unref?: () => void })?.unref?.();
}

function stopDeletedChatSweep(): void {
  if (deletedChatSweepTimer) {
    clearInterval(deletedChatSweepTimer);
    deletedChatSweepTimer = undefined;
  }
}

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

  stopDeletedChatSweep();
  // Kill every tracked PTY so shells don't leak after the server exits.
  terminalPtyManager.killAll();
  await portRegistry.closeAll();
  // Project script processes are owned by the server and are stopped on shutdown.
  // The service is instantiated by routes; process cleanup is handled by each run's
  // explicit stop path until a shared singleton lifecycle hook is added.

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
  // Sweeps stale forwarded ports whose target stopped while the owning
  // terminal/job stayed alive; removals emit "change" for SSE clients.
  portRegistry.startReaper();
  // Permanently removes soft-deleted chats older than 7 days.
  startDeletedChatSweep();

  Bun.serve<TerminalSocketData | PortTunnelSocketData>({
    port,
    hostname: host,
    // SSE agent-run streams can sit silent for minutes while a tool executes.
    // Bun's default idle timeout (10s) kills such connections mid-run — the
    // client then sees "SSE stream error" while the server keeps running the
    // agent (sidebar stays "Working"). 0 disables the timeout, matching the
    // old node:http bridging behavior. Tunnels are also long-lived.
    idleTimeout: 0,
    fetch(req, server) {
      if (isPortTunnelUpgradeRequest(req)) {
        const port = parseTunnelPort(req.url);
        if (port === null) return new Response("Invalid tunnel port", { status: 400 });
        const upgraded = server.upgrade(req, {
          data: { kind: "tunnel", port, url: req.url } as PortTunnelSocketData,
        });
        if (upgraded) return undefined;
        return new Response("Port tunnel upgrade failed", { status: 400 });
      }
      if (isTerminalUpgradeRequest(req)) {
        // Hijack the socket; handlers take over once the upgrade completes.
        const upgraded = server.upgrade(req, { data: { url: req.url, sessionId: null, paused: false, binary: false } });
        if (upgraded) return undefined;
        return new Response("Terminal WebSocket upgrade failed", { status: 400 });
      }
      return app.fetch(req);
    },
    websocket: {
      // Single Bun.serve multiplexes terminal + port-tunnel sockets.
      // Dispatch on the `kind` attached at upgrade time.
      data: {} as TerminalSocketData | PortTunnelSocketData,
      open(ws) {
        if ((ws.data as { kind?: string }).kind === "tunnel") {
          return portTunnelWebsocketHandlers.open(
            ws as import("bun").ServerWebSocket<PortTunnelSocketData>,
          );
        }
        return terminalWebsocketHandlers.websocket.open(
          ws as import("bun").ServerWebSocket<TerminalSocketData>,
        );
      },
      message(ws, data) {
        if ((ws.data as { kind?: string }).kind === "tunnel") {
          return portTunnelWebsocketHandlers.message(
            ws as import("bun").ServerWebSocket<PortTunnelSocketData>,
            data,
          );
        }
        return terminalWebsocketHandlers.websocket.message(
          ws as import("bun").ServerWebSocket<TerminalSocketData>,
          data,
        );
      },
      close(ws) {
        if ((ws.data as { kind?: string }).kind === "tunnel") {
          return portTunnelWebsocketHandlers.close(
            ws as import("bun").ServerWebSocket<PortTunnelSocketData>,
          );
        }
        return terminalWebsocketHandlers.websocket.close(
          ws as import("bun").ServerWebSocket<TerminalSocketData>,
        );
      },
    },
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
