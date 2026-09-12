import { EventEmitter } from "node:events";

const DEFAULT_PROXY_START = 45_000;
const DEFAULT_PROXY_END = 45_999;
const CANDIDATE_PATTERN = /(?:https?:\/\/)?(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::|\s*:\s*)(\d{1,5})/gi;
const ANSI_PATTERN = /\u001b\[[0-?]*[ -/]*[@-~]/g;
const PROBE_TIMEOUT_MS = 2_000;
const REAPER_INTERVAL_MS = (() => {
  const parsed = Number.parseInt(process.env.PORT_REAPER_INTERVAL_MS ?? "", 10);
  return Number.isInteger(parsed) && parsed >= 1_000 && parsed <= 60_000 ? parsed : 5_000;
})();

type Owner = { kind: "terminal" | "job"; id: string };

interface PortEntry {
  port: number;
  proxyPort: number;
  owner?: Owner;
  manual: boolean;
  server: Bun.Server<ProxySocketData>;
}

interface ProxySocketData {
  targetUrl: string;
  upstream?: WebSocket;
}

export interface ClientPort {
  port: number;
  url: string;
}

function parseRange(value: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(value ?? "", 10);
  return Number.isInteger(parsed) && parsed >= 1024 && parsed <= 65535 ? parsed : fallback;
}

function proxyHostFromRequest(request: Request): string {
  const host = request.headers.get("host") ?? "localhost";
  return host.replace(/:\d+$/, "");
}

function stripHopByHopHeaders(headers: Headers): Headers {
  const result = new Headers(headers);
  for (const name of [
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
  ]) {
    result.delete(name);
  }
  return result;
}

export class PortRegistry extends EventEmitter {
  private readonly entries = new Map<number, PortEntry>();
  private readonly ownerBuffers = new Map<string, string>();
  private readonly ownerEpochs = new Map<string, number>();
  private reaper: ReturnType<typeof setInterval> | undefined;
  private reaping = false;
  private readonly proxyStart = parseRange(process.env.PROXY_PORT_START, DEFAULT_PROXY_START);
  private readonly proxyEnd = parseRange(process.env.PROXY_PORT_END, DEFAULT_PROXY_END);
  private nextProxyPort = this.proxyStart;
  // Rebuilt only when entries mutate. The stream path must never call list(),
  // since list() performs liveness probes.
  private cachedSnapshot: ClientPort[] = [];

  async observeOutput(owner: Owner, chunk: Uint8Array | string): Promise<void> {
    const key = `${owner.kind}:${owner.id}`;
    const text = typeof chunk === "string" ? chunk : new TextDecoder().decode(chunk);
    const combined = (this.ownerBuffers.get(key) ?? "") + text.replace(ANSI_PATTERN, "");
    const lines = combined.split(/\r?\n/);
    this.ownerBuffers.set(key, lines.pop() ?? "");

    const candidates = new Set<number>();
    for (const line of lines) {
      CANDIDATE_PATTERN.lastIndex = 0;
      let match: RegExpExecArray | null;
      while ((match = CANDIDATE_PATTERN.exec(line))) {
        const port = Number.parseInt(match[1]!, 10);
        if (port >= 1_024 && port <= 65_535) candidates.add(port);
      }
    }
    await Promise.all([...candidates].map((port) => this.registerDetected(port, owner)));
  }

  async forward(port: number): Promise<ClientPort> {
    this.validatePort(port);
    if (!(await this.isListening(port))) {
      throw new Error(`Port ${port} is not listening on 127.0.0.1.`);
    }
    const existing = this.entries.get(port);
    if (existing) return this.clientEntry(existing, "localhost");
    return this.createEntry(port, { manual: true });
  }

  async list(host: string): Promise<ClientPort[]> {
    const entries = [...this.entries.values()];
    const availability = await Promise.all(
      entries.map(async (entry) => ({
        entry,
        listening: await this.isListening(entry.port),
      })),
    );

    for (const { entry, listening } of availability) {
      if (!listening && this.entries.get(entry.port) === entry) {
        await this.remove(entry.port);
      }
    }

    return [...this.entries.values()]
      .sort((a, b) => a.port - b.port)
      .map((entry) => this.clientEntry(entry, host));
  }

  async remove(port: number): Promise<boolean> {
    const entry = this.entries.get(port);
    if (!entry) return false;
    this.entries.delete(port);
    this.rebuildSnapshot();
    await entry.server.stop(true);
    this.emit("change", this.cachedSnapshot);
    return true;
  }

  async removeOwner(owner: Owner): Promise<void> {
    const doomed = [...this.entries.values()].filter(
      (entry) => entry.owner?.kind === owner.kind && entry.owner.id === owner.id,
    );
    for (const entry of doomed) await this.remove(entry.port);
    const key = `${owner.kind}:${owner.id}`;
    this.ownerBuffers.delete(key);
    // Bump the epoch so any in-flight observeOutput → registerDetected probes
    // for this owner cannot resurrect entries after cleanup.
    this.ownerEpochs.set(key, (this.ownerEpochs.get(key) ?? 0) + 1);
  }

  async closeAll(): Promise<void> {
    for (const port of [...this.entries.keys()]) await this.remove(port);
  }

  /** Start the background liveness sweep. Idempotent; safe to call twice. */
  startReaper(intervalMs = REAPER_INTERVAL_MS): void {
    if (this.reaper) return;
    // One shared server-side sweep for all SSE clients. Without this, a dev
    // server stopped inside a still-open terminal leaves a stale entry: no
    // owner exit fires, so nothing emits "change" and the SSE stream never
    // pushes the removal.
    this.reaper = setInterval(() => {
      void this.reap().catch(() => {});
    }, intervalMs);
    (this.reaper as unknown as { unref?: () => void })?.unref?.();
  }

  stopReaper(): void {
    if (this.reaper) {
      clearInterval(this.reaper);
      this.reaper = undefined;
    }
  }

  /** Run one liveness sweep immediately. Used by tests; the reaper calls reap() on a timer. */
  async sweepOnce(): Promise<void> {
    await this.reap();
  }

  private async reap(): Promise<void> {
    if (this.reaping || this.entries.size === 0) return;
    this.reaping = true;
    try {
      const entries = [...this.entries.values()];
      const availability = await Promise.all(
        entries.map(async (entry) => ({
          entry,
          listening: await this.isListening(entry.port),
        })),
      );
      for (const { entry, listening } of availability) {
        if (!listening && this.entries.get(entry.port) === entry) {
          await this.remove(entry.port);
        }
      }
    } finally {
      this.reaping = false;
    }
  }

  private async registerDetected(port: number, owner: Owner): Promise<void> {
    this.validatePort(port);
    const key = `${owner.kind}:${owner.id}`;
    const epoch = this.ownerEpochs.get(key) ?? 0;
    const existing = this.entries.get(port);
    if (existing) return;
    if (!(await this.isListening(port))) return;
    // The owner may have exited (or been killed) while the probe was in
    // flight. Re-check the epoch so a late probe cannot recreate an entry
    // that removeOwner() just cleaned up.
    if ((this.ownerEpochs.get(key) ?? 0) !== epoch) return;
    if (this.entries.get(port)) return;
    await this.createEntry(port, { owner, manual: false });
  }

  private async createEntry(port: number, options: { owner?: Owner; manual: boolean }): Promise<ClientPort> {
    const proxyPort = this.allocateProxyPort();
    const targetUrl = `http://127.0.0.1:${port}`;
    const server = Bun.serve<ProxySocketData>({
      port: proxyPort,
      hostname: process.env.PROXY_HOST ?? "0.0.0.0",
      fetch: async (request, serverInstance) => {
        if (request.headers.get("upgrade")?.toLowerCase() === "websocket") {
          const upgraded = serverInstance.upgrade(request, {
            data: { targetUrl: `${targetUrl}${new URL(request.url).pathname}${new URL(request.url).search}` },
          });
          return upgraded ? undefined : new Response("WebSocket upgrade failed", { status: 400 });
        }
        return this.proxyHttp(request, targetUrl);
      },
      websocket: {
        data: {} as ProxySocketData,
        open: (ws) => {
          const upstream = new WebSocket(ws.data.targetUrl);
          ws.data.upstream = upstream;
          upstream.binaryType = "arraybuffer";
          upstream.onopen = () => {};
          upstream.onmessage = (event) => ws.send(event.data as string | ArrayBuffer);
          upstream.onerror = () => ws.close(1011, "Upstream WebSocket error");
          upstream.onclose = (event) => ws.close(event.code || 1000, event.reason);
        },
        message: (ws, message) => {
          if (ws.data.upstream?.readyState === WebSocket.OPEN) ws.data.upstream.send(message);
        },
        close: (ws) => {
          ws.data.upstream?.close();
        },
      },
    });
    const entry: PortEntry = { port, proxyPort, owner: options.owner, manual: options.manual, server };
    this.entries.set(port, entry);
    this.rebuildSnapshot();
    this.emit("opened", port);
    this.emit("change", this.cachedSnapshot);
    return this.clientEntry(entry, "localhost");
  }

  private async proxyHttp(request: Request, targetBase: string): Promise<Response> {
    const incoming = new URL(request.url);
    const target = `${targetBase}${incoming.pathname}${incoming.search}`;
    const headers = stripHopByHopHeaders(request.headers);
    headers.set("host", new URL(targetBase).host);
    try {
      const response = await fetch(target, {
        method: request.method,
        headers,
        body: request.method === "GET" || request.method === "HEAD" ? undefined : request.body,
        // @ts-expect-error Bun's fetch supports streaming request bodies.
        duplex: "half",
      });
      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: stripHopByHopHeaders(response.headers),
      });
    } catch {
      return new Response("Forwarded service unavailable", { status: 502 });
    }
  }

  private isListening(port: number): Promise<boolean> {
    return new Promise((resolve) => {
      const controller = new AbortController();
      const timer = setTimeout(() => {
        controller.abort();
        resolve(false);
      }, PROBE_TIMEOUT_MS);
      fetch(`http://127.0.0.1:${port}/`, { signal: controller.signal })
        .then(() => resolve(true))
        .catch(() => resolve(false))
        .finally(() => clearTimeout(timer));
    });
  }

  private allocateProxyPort(): number {
    for (let i = 0; i <= this.proxyEnd - this.proxyStart; i++) {
      const candidate = this.nextProxyPort;
      this.nextProxyPort = candidate >= this.proxyEnd ? this.proxyStart : candidate + 1;
      if ([...this.entries.values()].every((entry) => entry.proxyPort !== candidate)) return candidate;
    }
    throw new Error("No proxy ports available.");
  }

  private clientEntry(entry: PortEntry, host: string): ClientPort {
    return { port: entry.port, url: `http://${host}:${entry.proxyPort}/` };
  }

  /** Return the mutation-time snapshot without probing forwarded ports. */
  snapshot(host = "localhost"): ClientPort[] {
    return this.cachedSnapshot.map((entry) => this.clientEntry(this.entries.get(entry.port)!, host));
  }

  private rebuildSnapshot(): void {
    this.cachedSnapshot = [...this.entries.values()]
      .sort((a, b) => a.port - b.port)
      .map((entry) => this.clientEntry(entry, "localhost"));
  }

  private validatePort(port: number): void {
    if (!Number.isInteger(port) || port < 1_024 || port > 65_535) {
      throw new Error("Port must be an integer between 1024 and 65535.");
    }
  }
}

export const portRegistry = new PortRegistry();
