import { create } from "zustand";

export type MobileTab = "home" | "chat" | "settings" | "terminal";

interface AppState {
  activeTab: MobileTab;
  previousTab: MobileTab | null;
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  backendUrl: string | null;
  pendingConnectionSection: boolean;
  setActiveTab: (tab: MobileTab) => void;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setBackendUrl: (url: string | null) => void;
  setPendingConnectionSection: (pending: boolean) => void;
}

export const useAppStore = create<AppState>((set) => ({
  activeTab: "home",
  previousTab: null,
  selectedProjectId: null,
  selectedSessionId: null,
  backendUrl: null,
  pendingConnectionSection: false,
  setActiveTab: (activeTab) =>
    set((state) =>
      state.activeTab === activeTab
        ? { activeTab }
        : { activeTab, previousTab: state.activeTab },
    ),
  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setBackendUrl: (backendUrl) => set({ backendUrl }),
  setPendingConnectionSection: (pendingConnectionSection) => set({ pendingConnectionSection }),
}));
