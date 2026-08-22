import { create } from "zustand";

export type MobileTab = "home" | "chat" | "settings" | "terminal";

interface AppState {
  activeTab: MobileTab;
  /** Tab shown before the current one — used by screens whose "back" should
   * return wherever the user came from (e.g. terminal entered from chat). */
  previousTab: MobileTab | null;
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
  previousTab: null,
  selectedProjectId: null,
  selectedSessionId: null,
  backendUrl: null,
  // Remember the outgoing tab so screens can navigate back to where the user
  // actually came from (self-transitions don't overwrite it).
  setActiveTab: (activeTab) =>
    set((state) =>
      state.activeTab === activeTab
        ? { activeTab }
        : { activeTab, previousTab: state.activeTab },
    ),
  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setBackendUrl: (backendUrl) => set({ backendUrl }),
}));
