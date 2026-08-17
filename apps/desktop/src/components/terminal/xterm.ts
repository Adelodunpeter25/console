import { Terminal, type ITerminalOptions } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { CanvasAddon } from "@xterm/addon-canvas";
import "@xterm/xterm/css/xterm.css";

export const DEFAULT_TERMINAL_OPTIONS: ITerminalOptions = {
  fontSize: 13,
  fontFamily:
    '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
  cursorBlink: true,
  cursorStyle: "block",
  scrollback: 10000,
  convertEol: true,
  smoothScrollDuration: 0,
  scrollSensitivity: 1.5,
  allowProposedApi: true,
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

export interface XtermInstance {
  terminal: Terminal;
  fitAddon: FitAddon;
  open: (container: HTMLElement) => void;
  fit: () => { cols: number; rows: number } | undefined;
  writeChunk: (data: string) => void;
  dispose: () => void;
}

/**
 * Creates and configures a high-performance xterm.js instance with WebGL/Canvas acceleration
 * and adaptive write batching for 60fps streaming under heavy CLI throughput.
 */
export function createXtermInstance(options: Partial<ITerminalOptions> = {}): XtermInstance {
  const terminal = new Terminal({
    ...DEFAULT_TERMINAL_OPTIONS,
    ...options,
  });

  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  let buffer: string[] = [];
  let bufferLength = 0;
  let rafId: number | null = null;

  const flushBuffer = () => {
    if (buffer.length > 0) {
      const combined = buffer.join("");
      buffer = [];
      bufferLength = 0;
      terminal.write(combined);
    }
    rafId = null;
  };

  return {
    terminal,
    fitAddon,
    open: (container: HTMLElement) => {
      terminal.open(container);

      // Hardware-accelerated rendering: try WebGL first, fallback to Canvas
      try {
        const webglAddon = new WebglAddon();
        webglAddon.onContextLoss(() => {
          webglAddon.dispose();
          const canvasAddon = new CanvasAddon();
          terminal.loadAddon(canvasAddon);
        });
        terminal.loadAddon(webglAddon);
      } catch {
        try {
          const canvasAddon = new CanvasAddon();
          terminal.loadAddon(canvasAddon);
        } catch {
          // Default DOM renderer is used as fallback
        }
      }
    },
    fit: () => {
      try {
        fitAddon.fit();
        return { cols: terminal.cols, rows: terminal.rows };
      } catch {
        return undefined;
      }
    },
    writeChunk: (data: string) => {
      buffer.push(data);
      bufferLength += data.length;

      // If buffer exceeds 32KB during high-throughput output, flush immediately
      if (bufferLength >= 32768) {
        if (rafId !== null) {
          cancelAnimationFrame(rafId);
          rafId = null;
        }
        flushBuffer();
        return;
      }

      if (rafId === null) {
        rafId = requestAnimationFrame(flushBuffer);
      }
    },
    dispose: () => {
      if (rafId !== null) {
        cancelAnimationFrame(rafId);
        rafId = null;
      }
      buffer = [];
      bufferLength = 0;
      try {
        fitAddon.dispose();
      } catch {}
      try {
        terminal.dispose();
      } catch {}
    },
  };
}
