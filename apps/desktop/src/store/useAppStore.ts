import { create } from "zustand";

export type DesktopView = "home" | "chat" | "settings";

interface AppState {
  activeView: DesktopView;
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  sidebarOpen: boolean;
  setActiveView: (view: DesktopView) => void;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
}

export const useAppStore = create<AppState>((set) => ({
  activeView: "home",
  selectedProjectId: null,
  selectedSessionId: null,
  sidebarOpen: true,
  setActiveView: (activeView) => set({ activeView }),
  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
  toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
}));
