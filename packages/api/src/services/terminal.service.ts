import type {
  TerminalClientMessage,
  TerminalServerMessage,
  TerminalSpawnParams,
  TerminalSpawnedEvent,
} from "@console/types";

/**
 * Interactive terminal WebSocket client for mobile/web clients that talk to
 * the Console server directly (the desktop app uses the Rust relay commands
 * in `src-tauri` instead).
 *
 * Protocol:
 *   - Connect to `{baseUrl}/api/terminals?cwd=...&cols=...&rows=...`
 *   - Server → client: JSON `TerminalServerMessage` frames
 *   - Client → server: JSON `TerminalClientMessage` frames
 */
export function connectTerminal(
  options: {
    baseUrl: string;
    params: TerminalSpawnParams;
    onEvent: (message: TerminalServerMessage) => void;
    onClose?: () => void;
    onError?: (message: string) => void;
  },
): { open: () => Promise<TerminalSpawnedEvent>; input: (data: string) => void; resize: (cols: number, rows: number) => void; kill: () => void; close: () => void } {
  const wsBase = options.baseUrl.replace(/^http/, "ws");
  const cwd = encodeURIComponent(options.params.cwd);
  const cols = options.params.cols ?? 80;
  const rows = options.params.rows ?? 24;
  const shell = options.params.shell
    ? `&shell=${encodeURIComponent(options.params.shell)}`
    : "";
  const label = options.params.label
    ? `&label=${encodeURIComponent(options.params.label)}`
    : "";

  const url = `${wsBase}/api/terminals?cwd=${cwd}&cols=${cols}&rows=${rows}${shell}${label}`;
  let ws: WebSocket | null = null;
  let spawnedPromise: Promise<TerminalSpawnedEvent> | null = null;

  const send = (message: TerminalClientMessage) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(message));
    }
  };

  const open = (): Promise<TerminalSpawnedEvent> => {
    if (spawnedPromise) return spawnedPromise;

    spawnedPromise = new Promise<TerminalSpawnedEvent>((resolve, reject) => {
      const socket = new WebSocket(url);
      ws = socket;
      let settled = false;

      socket.onopen = () => {
        // The server sends the "spawned" confirmation first.
      };

      socket.onmessage = (event) => {
        let message: TerminalServerMessage;
        try {
          message = JSON.parse(String(event.data)) as TerminalServerMessage;
        } catch {
          options.onError?.("Invalid terminal frame from server.");
          return;
        }

        if (!settled && message.type === "spawned") {
          settled = true;
          resolve(message);
        } else if (!settled && message.type === "error") {
          settled = true;
          reject(new Error(message.message));
          socket.close();
          return;
        }
        options.onEvent(message);
      };

      socket.onerror = () => {
        if (!settled) {
          settled = true;
          reject(new Error("Terminal WebSocket connection failed."));
        }
        options.onError?.("Terminal WebSocket connection failed.");
      };

      socket.onclose = () => {
        options.onClose?.();
      };
    });

    return spawnedPromise;
  };

  return {
    open,
    input: (data) => send({ type: "input", data }),
    resize: (cols, rows) => send({ type: "resize", cols, rows }),
    kill: () => send({ type: "kill" }),
    close: () => {
      if (ws) ws.close();
    },
  };
}