import { create } from "zustand";

interface ConsoleState {
  activeProjectId: string | null;
  activeSessionId: string | null;
  isSidebarOpen: boolean;
  setActiveProjectId: (id: string | null) => void;
  setActiveSessionId: (id: string | null) => void;
  setIsSidebarOpen: (open: boolean) => void;
}

export const useConsoleStore = create<ConsoleState>((set) => ({
  activeProjectId: null,
  activeSessionId: null,
  isSidebarOpen: true,
  setActiveProjectId: (id) => set({ activeProjectId: id }),
  setActiveSessionId: (id) => set({ activeSessionId: id }),
  setIsSidebarOpen: (open) => set({ isSidebarOpen: open }),
}));
