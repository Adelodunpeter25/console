import { useEffect, useRef, useState } from "react";
import type { TerminalServerMessage } from "@console/types";
import { useValue } from "@legendapp/state/react";
import {
  kill,
  openTerminal,
  resize,
  subscribe as subscribeTerminal,
  terminals$,
  write,
} from "@/stores/useTerminalStore";

/**
 * Subscribe a component to a terminal's server events (output/exit/error).
 * Returns the latest accumulated output buffer and the terminal status.
 */
export function useTerminalOutput(terminalId: string | undefined) {
  const terminal = useValue(() => (terminalId ? terminals$[terminalId].get() : undefined));
  const [output, setOutput] = useState("");
  const outputRef = useRef("");

  useEffect(() => {
    if (!terminalId) return;
    outputRef.current = "";
    setOutput("");
    const unsub = subscribeTerminal(terminalId, (message: TerminalServerMessage) => {
      if (message.type === "output") {
        outputRef.current += message.data;
        setOutput(outputRef.current);
      }
    });
    return unsub;
  }, [terminalId]);

  return {
    terminal,
    output,
    status: terminal?.status ?? "spawning",
    revision: terminal?.revision ?? 0,
  };
}

/** Open a terminal for a project and return controls + live output. */
export function useTerminal(opts: {
  projectId: string;
  cwd: string;
  cols?: number;
  rows?: number;
  label?: string;
  shell?: string;
}) {
  const [terminalId, setTerminalId] = useState<string | null>(null);

  const open = async () => {
    const spawned = await openTerminal(opts);
    setTerminalId(spawned.id);
    return spawned;
  };

  const output = useTerminalOutput(terminalId ?? undefined);

  return {
    terminalId,
    open,
    write: (data: string) => {
      if (terminalId) write(terminalId, data);
    },
    resizeTerminal: (cols: number, rows: number) => {
      if (terminalId) resize(terminalId, cols, rows);
    },
    kill: async () => {
      if (terminalId) await kill(terminalId);
    },
    ...output,
  };
}
