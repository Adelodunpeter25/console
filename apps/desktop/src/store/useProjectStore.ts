import { create } from "zustand";
import type { ProjectInfo, SessionHeader, UpdateSessionDto } from "@console/types";
import { api } from "../lib/api";
import { useSessionStatusStore } from "./useSessionStatusStore";

interface ProjectState {
  projects: ProjectInfo[];
  loading: boolean;
  /** Flat list of all sessions across projects, newest first. */
  sessions: SessionHeader[];
  sessionsLoading: boolean;
  deletedSessions: SessionHeader[];
  deletedSessionsLoading: boolean;
  loadProjects: () => Promise<void>;
  addProject: (path: string) => Promise<ProjectInfo>;
  loadSessions: () => Promise<void>;
  loadDeletedSessions: () => Promise<void>;
  createSession: (cwd: string, projectId: string, title?: string) => Promise<SessionHeader>;
  updateSession: (id: string, dto: UpdateSessionDto) => Promise<SessionHeader>;
  deleteSession: (id: string) => Promise<void>;
  restoreSession: (id: string) => Promise<void>;
  permanentlyDeleteSession: (id: string) => Promise<void>;
  /** Re-fetch a session header from the backend and patch it in-place (e.g. after an auto-renamed title). */
  refreshSessionHeader: (sessionId: string) => Promise<void>;
}

export const useProjectStore = create<ProjectState>((set) => ({
  projects: [],
  loading: false,
  sessions: [],
  sessionsLoading: false,
  deletedSessions: [],
  deletedSessionsLoading: false,

  loadProjects: async () => {
    set({ loading: true });
    try {
      const projects = await api.listProjects();
      set({ projects, loading: false });
    } catch {
      set({ loading: false });
    }
  },

  addProject: async (path: string) => {
    const project = await api.addProject(path);
    set((s) => ({ projects: [...s.projects, project] }));
    return project;
  },

  loadSessions: async () => {
    set({ sessionsLoading: true });
    try {
      const sessions = await api.listSessions();
      set({ sessions, sessionsLoading: false });
      useSessionStatusStore.getState().setStatuses(sessions);
    } catch {
      set({ sessionsLoading: false });
    }
  },

  loadDeletedSessions: async () => {
    set({ deletedSessionsLoading: true });
    try {
      const deletedSessions = await api.listSessions(undefined, undefined, true);
      set({ deletedSessions, deletedSessionsLoading: false });
    } catch {
      set({ deletedSessionsLoading: false });
    }
  },

  createSession: async (cwd: string, projectId: string, title?: string) => {
    const session = await api.createSession({
      cwd,
      projectId,
      title: title ?? "New Chat",
    });
    set((s) => ({ sessions: [session, ...s.sessions] }));
    useSessionStatusStore.getState().setStatus(session.id, session.status ?? "idle");
    return session;
  },

  updateSession: async (id, dto) => {
    const updated = await api.updateSession(id, dto);
    set((s) => ({
      sessions: s.sessions.map((sess) => (sess.id === id ? { ...sess, ...updated } : sess)),
    }));
    return updated;
  },

  deleteSession: async (id: string) => {
    await api.deleteSession(id);
    set((s) => ({ sessions: s.sessions.filter((sess) => sess.id !== id) }));
    useSessionStatusStore.getState().clearStatus(id);
  },

  restoreSession: async (id: string) => {
    await api.restoreSession(id);
    set((s) => {
      const restored = s.deletedSessions.find((sess) => sess.id === id);
      const filteredDeleted = s.deletedSessions.filter((sess) => sess.id !== id);
      if (restored) {
        return {
          deletedSessions: filteredDeleted,
          sessions: [restored, ...s.sessions],
        };
      }
      return { deletedSessions: filteredDeleted };
    });
  },

  permanentlyDeleteSession: async (id: string) => {
    await api.permanentlyDeleteSession(id);
    set((s) => ({ deletedSessions: s.deletedSessions.filter((sess) => sess.id !== id) }));
  },

  refreshSessionHeader: async (sessionId) => {
    try {
      const detail = await api.getSession(sessionId);
      const header = detail.header;
      set((s) => ({
        sessions: s.sessions.map((sess) => (sess.id === sessionId ? { ...sess, ...header } : sess)),
      }));
      if (header.status) useSessionStatusStore.getState().setStatus(sessionId, header.status);
    } catch {
      // Ignore refresh failures — the header will update on next load.
    }
  },
}));
