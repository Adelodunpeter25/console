import { create } from "zustand";
import type { ProjectInfo, SessionHeader, UpdateSessionDto } from "@console/types";
import { sessionService, fsService, sessionKeys, fsKeys } from "@console/api";
import { queryClient } from "../query-client";
import { useSessionStatusStore } from "./useSessionStatusStore";
import { useAppStore } from "./useAppStore";

interface ProjectState {
  projects: ProjectInfo[];
  loading: boolean;
  /** Flat list of all sessions across projects, newest first. */
  sessions: SessionHeader[];
  sessionsLoading: boolean;
  deletedSessions: SessionHeader[];
  deletedSessionsLoading: boolean;
  loadProjects: () => Promise<ProjectInfo[]>;
  addProject: (path: string) => Promise<ProjectInfo>;
  deleteProject: (projectId: string) => Promise<void>;
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
      const projects = await fsService.getProjects();
      set({ projects, loading: false });
      return projects;
    } catch {
      set({ loading: false });
      return [];
    }
  },

  addProject: async (path: string) => {
    const project = await fsService.addProject(path);
    set((s) => ({ projects: [...s.projects, project] }));
    queryClient.invalidateQueries({ queryKey: fsKeys.projects }).catch(() => {});
    return project;
  },

  deleteProject: async (projectId: string) => {
    await fsService.deleteProject(projectId);
    set((s) => ({ projects: s.projects.filter((p) => p.id !== projectId) }));
    queryClient.invalidateQueries({ queryKey: fsKeys.projects }).catch(() => {});
    // A deleted project must never keep a dangling selection (the terminal
    // would otherwise show a stale working directory).
    if (useAppStore.getState().selectedProjectId === projectId) {
      useAppStore.getState().setSelectedProjectId(null);
    }
  },

  loadSessions: async () => {
    set({ sessionsLoading: true });
    try {
      const sessions = await sessionService.getSessions();
      set({ sessions, sessionsLoading: false });
      useSessionStatusStore.getState().setStatuses(sessions);
    } catch {
      set({ sessionsLoading: false });
    }
  },

  loadDeletedSessions: async () => {
    set({ deletedSessionsLoading: true });
    try {
      const deletedSessions = await sessionService.getSessions({ onlyDeleted: true });
      set({ deletedSessions, deletedSessionsLoading: false });
    } catch {
      set({ deletedSessionsLoading: false });
    }
  },

  createSession: async (cwd: string, projectId: string, title?: string) => {
    const session = await sessionService.createSession({
      cwd,
      projectId,
      title: title ?? "New Chat",
    });
    set((s) => ({ sessions: [session, ...s.sessions] }));
    useSessionStatusStore.getState().setStatus(session.id, session.status ?? "idle");
    queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
    return session;
  },

  updateSession: async (id, dto) => {
    const updated = await sessionService.updateSession(id, dto);
    set((s) => ({
      sessions: s.sessions.map((sess) => (sess.id === id ? { ...sess, ...updated } : sess)),
    }));
    queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
    return updated;
  },

  deleteSession: async (id: string) => {
    await sessionService.deleteSession(id);
    set((s) => ({ sessions: s.sessions.filter((sess) => sess.id !== id) }));
    useSessionStatusStore.getState().clearStatus(id);
    queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  },

  restoreSession: async (id: string) => {
    await sessionService.restoreSession(id);
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
    queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  },

  permanentlyDeleteSession: async (id: string) => {
    await sessionService.permanentlyDeleteSession(id);
    set((s) => ({
      deletedSessions: s.deletedSessions.filter((sess) => sess.id !== id),
    }));
    queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  },

  refreshSessionHeader: async (sessionId) => {
    try {
      const detail = await sessionService.getSession(sessionId);
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

