import { create } from "zustand";
import type { ProjectInfo, SessionHeader, SessionStatus, UpdateSessionDto } from "@console/types";
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
  updateSession: (
    id: string,
    projectId: string,
    dto: UpdateSessionDto,
  ) => Promise<SessionHeader>;
  deleteSession: (id: string, projectId: string) => Promise<void>;
  /** Update a session's status in-place across all project buckets. */
  updateSessionStatus: (sessionId: string, status: SessionStatus) => void;
  /** Re-fetch a session header from the backend and patch it in-place (e.g. after an auto-renamed title). */
  refreshSessionHeader: (sessionId: string) => Promise<void>;
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

  updateSession: async (id, projectId, dto) => {
    const updated = await tauriApi.updateSession(id, dto);
    set((s) => ({
      sessionsByProject: {
        ...s.sessionsByProject,
        [projectId]: (s.sessionsByProject[projectId] ?? []).map((sess) =>
          sess.id === id ? updated : sess,
        ),
      },
    }));
    return updated;
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

  updateSessionStatus: (sessionId, status) => {
    set((s) => {
      const updated: Record<string, SessionHeader[]> = {};
      let changed = false;
      for (const [pid, sessions] of Object.entries(s.sessionsByProject)) {
        const idx = sessions.findIndex((sess) => sess.id === sessionId);
        if (idx >= 0) {
          changed = true;
          updated[pid] = sessions.map((sess) =>
            sess.id === sessionId ? { ...sess, status } : sess,
          );
        }
      }
      return changed
        ? { sessionsByProject: { ...s.sessionsByProject, ...updated } }
        : s;
    });
  },

  refreshSessionHeader: async (sessionId) => {
    try {
      const detail = await tauriApi.getSession(sessionId);
      const header = detail.header;
      set((s) => {
        const updated: Record<string, SessionHeader[]> = {};
        let changed = false;
        for (const [pid, sessions] of Object.entries(s.sessionsByProject)) {
          if (sessions.some((sess) => sess.id === sessionId)) {
            changed = true;
            updated[pid] = sessions.map((sess) =>
              sess.id === sessionId ? { ...sess, ...header } : sess,
            );
          }
        }
        return changed
          ? { sessionsByProject: { ...s.sessionsByProject, ...updated } }
          : s;
      });
    } catch {
      // Ignore refresh failures — the header will update on next load.
    }
  },
}));
