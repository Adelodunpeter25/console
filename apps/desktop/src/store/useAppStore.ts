import { create } from "zustand";
import { setSidebarOpen as persistSidebarOpen, setRightSidebarOpen as persistRightSidebarOpen } from "../lib/ui-store";
import type { TerminalTabConfig } from "../layout/types";

interface AppState {
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  sidebarOpen: boolean;
  rightSidebarOpen: boolean;
  commandPaletteOpen: boolean;
  dockedTerminal: TerminalTabConfig | null;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  setRightSidebarOpen: (open: boolean) => void;
  toggleRightSidebar: () => void;
  setCommandPaletteOpen: (open: boolean) => void;
  setDockedTerminal: (terminal: TerminalTabConfig | null) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  selectedProjectId: null,
  selectedSessionId: null,
  sidebarOpen: true,
  rightSidebarOpen: true,
  commandPaletteOpen: false,
  dockedTerminal: null,

  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setSidebarOpen: (sidebarOpen) => {
    set({ sidebarOpen });
    persistSidebarOpen(sidebarOpen).catch(() => {});
  },
  toggleSidebar: () => {
    const next = !get().sidebarOpen;
    set({ sidebarOpen: next });
    persistSidebarOpen(next).catch(() => {});
  },
  setRightSidebarOpen: (rightSidebarOpen) => {
    set({ rightSidebarOpen });
    persistRightSidebarOpen(rightSidebarOpen).catch(() => {});
  },
  toggleRightSidebar: () => {
    const next = !get().rightSidebarOpen;
    set({ rightSidebarOpen: next });
    persistRightSidebarOpen(next).catch(() => {});
  },
  setCommandPaletteOpen: (commandPaletteOpen) => set({ commandPaletteOpen }),
  setDockedTerminal: (dockedTerminal) => set({ dockedTerminal }),
}));
