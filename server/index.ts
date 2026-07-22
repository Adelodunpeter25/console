/**
 * Server Entry Point — Starts Hono API Server listening on 0.0.0.0:3000.
 * Supports daemon mode with logging and graceful shutdown.
 */
import { createServer } from "node:http";
import { createApiApp } from "./api/src/app.js";
import * as fs from "node:fs/promises";
import * as path from "node:path";

const app = createApiApp();
const port = Number.parseInt(process.env.PORT || "3000", 10);
const host = process.env.HOST || "0.0.0.0";
const isDaemon = process.env.CONSOLE_DAEMON === "true";

// Logging setup
let logFile: string | null = null;
let logStream: fs.FileHandle | null = null;

async function setupLogging(): Promise<void> {
  if (!isDaemon) return;
  
  const consoleDir = path.join(process.env.HOME || process.env.USERPROFILE || "", ".console");
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
  log("🛑 Shutting down server...");
  
  // Close log stream
  if (logStream) {
    await logStream.close();
  }
  
  process.exit(0);
}

const server = createServer(async (req, res) => {
  const url = `http://${req.headers.host || "localhost"}${req.url}`;

  // Convert Node.js IncomingMessage to Web Standard Request
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (value !== undefined) {
      if (Array.isArray(value)) {
        for (const v of value) headers.append(key, v);
      } else {
        headers.set(key, value);
      }
    }
  }

  const method = req.method || "GET";
  let body: BodyInit | null = null;
  if (method !== "GET" && method !== "HEAD") {
    const chunks: Uint8Array[] = [];
    for await (const chunk of req) {
      chunks.push(chunk);
    }
    body = Buffer.concat(chunks);
  }

  const webReq = new Request(url, {
    method,
    headers,
    body,
  });

  const webRes = await app.fetch(webReq);

  res.statusCode = webRes.status;
  webRes.headers.forEach((val, key) => {
    res.setHeader(key, val);
  });

  if (webRes.body) {
    const reader = webRes.body.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      res.write(value);
    }
  }
  res.end();
});

// Graceful shutdown handlers
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);

// Start server
async function startServer(): Promise<void> {
  await setupLogging();
  
  server.listen(port, host, () => {
    log(`🚀 Console Agent Server running on http://${host}:${port}`);
    log(`📡 API Base: http://${host}:${port}/api (Accepting connections from all hosts/devices)`);
    log(`📝 Mode: ${isDaemon ? "daemon" : "foreground"}`);
    
    if (isDaemon) {
      log(`📁 Logs: ${logFile}`);
    }
  });
}

startServer().catch((error) => {
  console.error("Failed to start server:", error);
  process.exit(1);
});
