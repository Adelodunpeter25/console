import { create } from "zustand";
import type { TerminalTabConfig } from "../layout/types";

export const MAX_DOCKED_TERMINALS = 3;

interface AppState {
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  sidebarOpen: boolean;
  rightSidebarOpen: boolean;
  sidebarWidth: number;
  rightSidebarWidth: number;
  rightSidebarPanelSizes: [number, number];
  commandPaletteOpen: boolean;
  dockedTerminals: TerminalTabConfig[];
  activeDockedTerminalId: string | null;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  setRightSidebarOpen: (open: boolean) => void;
  toggleRightSidebar: () => void;
  setSidebarWidth: (width: number) => void;
  setRightSidebarWidth: (width: number) => void;
  setRightSidebarPanelSizes: (sizes: [number, number]) => void;
  setCommandPaletteOpen: (open: boolean) => void;
  dockTerminal: (terminal: TerminalTabConfig) => boolean;
  setActiveDockedTerminal: (terminalId: string) => void;
  removeDockedTerminal: (terminalId: string) => void;
  restoreDockedTerminals: (terminals: TerminalTabConfig[], activeTerminalId: string | null) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  selectedProjectId: null,
  selectedSessionId: null,
  sidebarOpen: true,
  rightSidebarOpen: true,
  sidebarWidth: 288,
  rightSidebarWidth: 288,
  rightSidebarPanelSizes: [55, 45],
  commandPaletteOpen: false,
  dockedTerminals: [],
  activeDockedTerminalId: null,

  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
  toggleSidebar: () => {
    const next = !get().sidebarOpen;
    set({ sidebarOpen: next });
  },
  setRightSidebarOpen: (rightSidebarOpen) => set({ rightSidebarOpen }),
  toggleRightSidebar: () => {
    const next = !get().rightSidebarOpen;
    set({ rightSidebarOpen: next });
  },
  setSidebarWidth: (sidebarWidth) => set({ sidebarWidth }),
  setRightSidebarWidth: (rightSidebarWidth) => set({ rightSidebarWidth }),
  setRightSidebarPanelSizes: (rightSidebarPanelSizes) =>
    set((state) =>
      state.rightSidebarPanelSizes[0] === rightSidebarPanelSizes[0] &&
      state.rightSidebarPanelSizes[1] === rightSidebarPanelSizes[1]
        ? state
        : { rightSidebarPanelSizes },
    ),
  setCommandPaletteOpen: (commandPaletteOpen) => set({ commandPaletteOpen }),
  dockTerminal: (terminal) => {
    let accepted = false;
    set((state) => {
      if (state.dockedTerminals.some((item) => item.terminalId === terminal.terminalId)) {
        accepted = true;
        return { activeDockedTerminalId: terminal.terminalId };
      }
      if (state.dockedTerminals.length >= MAX_DOCKED_TERMINALS) return state;

      const baseTitle = terminal.title ?? "Terminal";
      const titles = new Set(state.dockedTerminals.map((item) => item.title ?? "Terminal"));
      let title = baseTitle;
      let suffix = 2;
      while (titles.has(title)) {
        title = `${baseTitle} ${suffix}`;
        suffix += 1;
      }

      const nextTerminal = { ...terminal, title };
      accepted = true;
      return {
        dockedTerminals: [...state.dockedTerminals, nextTerminal],
        activeDockedTerminalId: terminal.terminalId,
      };
    });
    return accepted;
  },
  setActiveDockedTerminal: (terminalId) =>
    set((state) =>
      state.dockedTerminals.some((terminal) => terminal.terminalId === terminalId)
        ? { activeDockedTerminalId: terminalId }
        : state,
    ),
  removeDockedTerminal: (terminalId) =>
    set((state) => {
      const index = state.dockedTerminals.findIndex((terminal) => terminal.terminalId === terminalId);
      if (index < 0) return state;

      const dockedTerminals = state.dockedTerminals.filter(
        (terminal) => terminal.terminalId !== terminalId,
      );
      const activeDockedTerminalId =
        state.activeDockedTerminalId === terminalId
          ? (dockedTerminals[Math.max(0, index - 1)]?.terminalId ?? null)
          : state.activeDockedTerminalId;

      return { dockedTerminals, activeDockedTerminalId };
    }),
  restoreDockedTerminals: (terminals, activeTerminalId) => {
    const dockedTerminals = terminals.slice(0, MAX_DOCKED_TERMINALS);
    const activeDockedTerminalId = dockedTerminals.some(
      (terminal) => terminal.terminalId === activeTerminalId,
    )
      ? activeTerminalId
      : (dockedTerminals[0]?.terminalId ?? null);
    set({ dockedTerminals, activeDockedTerminalId });
  },
}));
