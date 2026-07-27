import { create } from "zustand";

interface AppState {
  selectedProjectId: string | null;
  selectedSessionId: string | null;
  sidebarOpen: boolean;
  /** Set of expanded project IDs in the sidebar. */
  expandedProjects: Set<string>;
  setSelectedProjectId: (id: string | null) => void;
  setSelectedSessionId: (id: string | null) => void;
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  toggleProjectExpanded: (projectId: string) => void;
  setProjectExpanded: (projectId: string, expanded: boolean) => void;
}

export const useAppStore = create<AppState>((set) => ({
  selectedProjectId: null,
  selectedSessionId: null,
  sidebarOpen: true,
  expandedProjects: new Set<string>(),

  setSelectedProjectId: (selectedProjectId) => set({ selectedProjectId }),
  setSelectedSessionId: (selectedSessionId) => set({ selectedSessionId }),
  setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
  toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),

  toggleProjectExpanded: (projectId) =>
    set((s) => {
      const next = new Set(s.expandedProjects);
      if (next.has(projectId)) {
        next.delete(projectId);
      } else {
        next.add(projectId);
      }
      return { expandedProjects: next };
    }),

  setProjectExpanded: (projectId, expanded) =>
    set((s) => {
      const next = new Set(s.expandedProjects);
      if (expanded) {
        next.add(projectId);
      } else {
        next.delete(projectId);
      }
      return { expandedProjects: next };
    }),
}));
