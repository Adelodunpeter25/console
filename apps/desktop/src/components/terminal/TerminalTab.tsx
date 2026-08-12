import React from "react";
import type { TerminalTabConfig } from "../../layout/types";
import { tauriApi } from "../../lib/tauri-api";
import { useTerminalStore } from "../../store/useTerminalStore";
import { createXtermInstance, type XtermInstance } from "./xterm";

interface TerminalTabProps {
  config: TerminalTabConfig;
}

/**
 * Interactive terminal tab — renders xterm.js and pipes bytes to/from the
 * server PTY through the Rust relay (tauriApi.terminal*).
 */
export function TerminalTab({ config }: TerminalTabProps) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const instanceRef = React.useRef<XtermInstance | null>(null);
  const resizeTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const markStatus = useTerminalStore((s) => s.markStatus);
  const write = useTerminalStore((s) => s.write);
  const resize = useTerminalStore((s) => s.resize);

  React.useEffect(() => {
    let disposed = false;
    let unlisten: (() => void) | null = null;
    let resizeObserver: ResizeObserver | null = null;

    if (!containerRef.current) return;

    const instance = createXtermInstance();
    instanceRef.current = instance;
    instance.open(containerRef.current);

    const fitAndSync = () => {
      if (!instanceRef.current) return;
      const dims = instanceRef.current.fit();
      if (dims && dims.cols > 0 && dims.rows > 0) {
        if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
        resizeTimerRef.current = setTimeout(() => {
          resize(config.terminalId, dims.cols, dims.rows);
        }, 50);
      }
    };

    instance.terminal.onData((data) => {
      write(config.terminalId, data);
    });

    void (async () => {
      unlisten = await tauriApi.listenTerminalEvents(config.terminalId, (message) => {
        if (message.type === "output") {
          instance.writeChunk(message.data);
        } else if (message.type === "exit") {
          markStatus(config.terminalId, "exited");
        } else if (message.type === "error") {
          markStatus(config.terminalId, "error", { error: message.message });
        }
      });

      if (disposed) {
        unlisten?.();
        instance.dispose();
        return;
      }

      markStatus(config.terminalId, "running");
      fitAndSync();
      if (containerRef.current) {
        resizeObserver = new ResizeObserver(() => fitAndSync());
        resizeObserver.observe(containerRef.current);
      }
      window.addEventListener("resize", fitAndSync);
    })();

    return () => {
      disposed = true;
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
      unlisten?.();
      resizeObserver?.disconnect();
      window.removeEventListener("resize", fitAndSync);
      instanceRef.current = null;
      instance.dispose();
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