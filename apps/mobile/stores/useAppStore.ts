import { create } from "zustand";

export type MobileTab = "home" | "chat" | "settings" | "terminal";

interface AppState {
  activeTab: MobileTab;
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  backendUrl: string | null;
  setActiveTab: (tab: MobileTab) => void;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setBackendUrl: (url: string | null) => void;
}

export const useAppStore = create<AppState>((set) => ({
  activeTab: "home",
  selectedProjectId: null,
  selectedSessionId: null,
  backendUrl: null,
  setActiveTab: (activeTab) => set({ activeTab }),
  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setBackendUrl: (backendUrl) => set({ backendUrl }),
}));
