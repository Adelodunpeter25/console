import React from "react";
import { FitAddon, init, Terminal, type ITerminalOptions } from "ghostty-web";
import type { TerminalTabConfig } from "../../layout/types";
import { tauriApi } from "../../lib/tauri-api";
import { useTerminalStore } from "../../store/useTerminalStore";

/** Shared init promise — the WASM module must be loaded exactly once. */
let initPromise: Promise<void> | null = null;
function ensureGhostty(): Promise<void> {
  if (!initPromise) initPromise = init();
  return initPromise;
}

const TERMINAL_OPTIONS: ITerminalOptions = {
  fontSize: 13,
  cursorBlink: true,
  cursorStyle: "block",
  theme: {
    background: "#0d0d0d",
    foreground: "#d4d4d4",
    black: "#000000",
    red: "#cd3131",
    green: "#0dbc79",
    yellow: "#e5e510",
    blue: "#2472c8",
    magenta: "#bc3fbc",
    cyan: "#11a8cd",
    white: "#e5e5e5",
    brightBlack: "#666666",
    brightRed: "#f14c4c",
    brightGreen: "#23d18b",
    brightYellow: "#f5f543",
    brightBlue: "#3b8eea",
    brightMagenta: "#d670d6",
    brightCyan: "#29b8db",
    brightWhite: "#ffffff",
  },
};

interface TerminalTabProps {
  config: TerminalTabConfig;
}

/**
 * Interactive terminal tab — renders ghostty-web and pipes bytes to/from the
 * server PTY through the Rust relay (tauriApi.terminal*).
 */
export function TerminalTab({ config }: TerminalTabProps) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const termRef = React.useRef<Terminal | null>(null);
  const fitRef = React.useRef<FitAddon | null>(null);
  const markStatus = useTerminalStore((s) => s.markStatus);
  const write = useTerminalStore((s) => s.write);
  const resize = useTerminalStore((s) => s.resize);
  const kill = useTerminalStore((s) => s.kill);

  // Boot wasm + terminal and subscribe to server output.
  React.useEffect(() => {
    let disposed = false;
    let unlisten: (() => void) | null = null;
    let resizeObserver: ResizeObserver | null = null;

    const fitAndSync = () => {
      const term = termRef.current;
      const fit = fitRef.current;
      if (!term || !fit) return;
      try {
        fit.fit();
      } catch {
        return;
      }
      const dims = fit.proposeDimensions();
      if (dims && dims.cols > 0 && dims.rows > 0) {
        resize(config.terminalId, dims.cols, dims.rows);
      }
      markStatus(config.terminalId, "running");
    };

    void (async () => {
      try {
        await ensureGhostty();
      } catch (err) {
        markStatus(config.terminalId, "error", {
          error: err instanceof Error ? err.message : String(err),
        });
        return;
      }
      if (disposed || !containerRef.current) return;

      const term = new Terminal(TERMINAL_OPTIONS);
      termRef.current = term;
      term.open(containerRef.current);

      // Single keystroke → server PTY route.
      term.onData((data) => {
        write(config.terminalId, data);
      });

      // Fit the terminal to its container and report dims to the server.
      const fit = new FitAddon();
      fitRef.current = fit;
      term.loadAddon(fit);

      // Subscribe to server → terminal output through the Rust relay.
      unlisten = await tauriApi.listenTerminalEvents(config.terminalId, (message) => {
        if (message.type === "output") {
          term.write(message.data);
        } else if (message.type === "exit") {
          markStatus(config.terminalId, "exited");
        } else if (message.type === "error") {
          markStatus(config.terminalId, "error", { error: message.message });
        }
      });

      if (disposed) {
        unlisten?.();
        term.dispose();
        return;
      }

      fitAndSync();
      resizeObserver = new ResizeObserver(() => fitAndSync());
      if (containerRef.current) {
        resizeObserver.observe(containerRef.current);
      }
      window.addEventListener("resize", fitAndSync);
    })();

    return () => {
      disposed = true;
      unlisten?.();
      resizeObserver?.disconnect();
      window.removeEventListener("resize", fitAndSync);
      fitRef.current = null;
      termRef.current?.dispose();
      termRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [config.terminalId, markStatus, resize, write]);

  const terminalRecord = useTerminalStore((s) => s.terminals[config.terminalId]);
  const isInactive = terminalRecord?.status === "exited" || terminalRecord?.status === "error";

  return (
    <div className="relative h-full w-full bg-[#0d0d0d]">
      {isInactive && (
        <div className="absolute top-0 left-0 right-0 z-10 flex items-center justify-between bg-neutral-900/90 px-4 py-2 text-xs text-neutral-400 backdrop-blur border-b border-neutral-800">
          <span>
            {terminalRecord?.status === "error"
              ? `Terminal error: ${terminalRecord.error ?? "Session failed"}`
              : "Terminal session ended"}
          </span>
        </div>
      )}
      <div
        ref={containerRef}
        className="h-full w-full overflow-hidden bg-[#0d0d0d] [&>div]:h-full"
      />
    </div>
  );
}