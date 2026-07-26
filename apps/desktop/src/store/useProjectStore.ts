import { create } from "zustand";
import type { ProjectInfo, SessionHeader } from "@console/types";
import { tauriApi } from "../lib/tauri-api";

interface ProjectState {
  projects: ProjectInfo[];
  loading: boolean;
  sessionsByProject: Record<string, SessionHeader[]>;
  loadProjects: () => Promise<void>;
  addProject: (path: string) => Promise<ProjectInfo>;
  loadSessions: (projectId: string) => Promise<void>;
  createSession: (
    cwd: string,
    projectId: string,
    title?: string,
  ) => Promise<SessionHeader>;
  deleteSession: (id: string, projectId: string) => Promise<void>;
}

export const useProjectStore = create<ProjectState>((set, get) => ({
  projects: [],
  loading: false,
  sessionsByProject: {},

  loadProjects: async () => {
    set({ loading: true });
    try {
      const projects = await tauriApi.listProjects();
      set({ projects, loading: false });
    } catch {
      set({ loading: false });
    }
  },

  addProject: async (path: string) => {
    const project = await tauriApi.addProject(path);
    set((s) => ({ projects: [...s.projects, project] }));
    return project;
  },

  loadSessions: async (projectId: string) => {
    try {
      const sessions = await tauriApi.listSessions(undefined, projectId);
      set((s) => ({
        sessionsByProject: {
          ...s.sessionsByProject,
          [projectId]: sessions,
        },
      }));
    } catch {
      // ignore
    }
  },

  createSession: async (cwd: string, projectId: string, title?: string) => {
    const session = await tauriApi.createSession({
      cwd,
      projectId,
      title: title ?? "New Chat",
    });
    set((s) => ({
      sessionsByProject: {
        ...s.sessionsByProject,
        [projectId]: [session, ...(s.sessionsByProject[projectId] ?? [])],
      },
    }));
    return session;
  },

  deleteSession: async (id: string, projectId: string) => {
    await tauriApi.deleteSession(id);
    set((s) => ({
      sessionsByProject: {
        ...s.sessionsByProject,
        [projectId]: (s.sessionsByProject[projectId] ?? []).filter(
          (sess) => sess.id !== id,
        ),
      },
    }));
  },
}));
