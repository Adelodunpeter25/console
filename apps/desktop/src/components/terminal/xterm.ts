import { Terminal, type ITerminalOptions } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export const DEFAULT_TERMINAL_OPTIONS: ITerminalOptions = {
  fontSize: 13,
  fontFamily:
    'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
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

export interface XtermInstance {
  terminal: Terminal;
  fitAddon: FitAddon;
  open: (container: HTMLElement) => void;
  fit: () => { cols: number; rows: number } | undefined;
  dispose: () => void;
}

/**
 * Creates and configures an xterm.js instance with FitAddon attached.
 */
export function createXtermInstance(options: Partial<ITerminalOptions> = {}): XtermInstance {
  const terminal = new Terminal({
    ...DEFAULT_TERMINAL_OPTIONS,
    ...options,
  });

  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);

  return {
    terminal,
    fitAddon,
    open: (container: HTMLElement) => {
      terminal.open(container);
    },
    fit: () => {
      try {
        fitAddon.fit();
        return { cols: terminal.cols, rows: terminal.rows };
      } catch {
        return undefined;
      }
    },
    dispose: () => {
      try {
        fitAddon.dispose();
      } catch {
        // ignore
      }
      try {
        terminal.dispose();
      } catch {
        // ignore
      }
    },
  };
}
