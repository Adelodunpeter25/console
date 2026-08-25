import { batch, observable } from "@legendapp/state";
import type { ProjectInfo, SessionHeader, UpdateSessionDto } from "@console/types";
import { sessionService, fsService, sessionKeys, fsKeys } from "@console/api";
import { queryClient } from "@/query-client";
import { clearStatus, setStatus, setStatuses } from "./useSessionStatusStore";
import { app$, setSelectedProjectId } from "./useAppStore";

/**
 * Projects + flat session lists as Legend State observables.
 * See docs/legend-state-and-list-migration.md.
 *
 * Reads in components subscribe via `useValue(project$.field)`;
 * imperative reads outside render use `.peek()`.
 */
export const project$ = observable({
  projects: [] as ProjectInfo[],
  loading: false,
  /** Flat list of all sessions across projects, newest first. */
  sessions: [] as SessionHeader[],
  sessionsLoading: false,
  deletedSessions: [] as SessionHeader[],
  deletedSessionsLoading: false,
});

export async function loadProjects(): Promise<ProjectInfo[]> {
  project$.loading.set(true);
  try {
    const projects = await fsService.getProjects();
    project$.projects.set(projects);
    project$.loading.set(false);
    return projects;
  } catch {
    project$.loading.set(false);
    return [];
  }
}

export async function addProject(path: string): Promise<ProjectInfo> {
  const project = await fsService.addProject(path);
  project$.projects.push(project);
  queryClient.invalidateQueries({ queryKey: fsKeys.projects }).catch(() => {});
  return project;
}

export async function deleteProject(projectId: string): Promise<void> {
  await fsService.deleteProject(projectId);
  project$.projects.set((prev) => prev.filter((p) => p.id !== projectId));
  queryClient.invalidateQueries({ queryKey: fsKeys.projects }).catch(() => {});
  // A deleted project must never keep a dangling selection (the terminal
  // would otherwise show a stale working directory).
  if (app$.selectedProjectId.peek() === projectId) {
    setSelectedProjectId(null);
  }
}

export async function loadSessions(): Promise<void> {
  project$.sessionsLoading.set(true);
  try {
    const sessions = await sessionService.getSessions();
    batch(() => {
      project$.sessions.set(sessions);
      project$.sessionsLoading.set(false);
    });
    setStatuses(sessions);
  } catch {
    project$.sessionsLoading.set(false);
  }
}

export async function loadDeletedSessions(): Promise<void> {
  project$.deletedSessionsLoading.set(true);
  try {
    const deletedSessions = await sessionService.getSessions({ onlyDeleted: true });
    batch(() => {
      project$.deletedSessions.set(deletedSessions);
      project$.deletedSessionsLoading.set(false);
    });
  } catch {
    project$.deletedSessionsLoading.set(false);
  }
}

export async function createSession(
  cwd: string,
  projectId: string,
  title?: string,
): Promise<SessionHeader> {
  const session = await sessionService.createSession({
    cwd,
    projectId,
    title: title ?? "New Chat",
  });
  project$.sessions.unshift(session);
  setStatus(session.id, session.status ?? "idle");
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  return session;
}

export async function updateSession(id: string, dto: UpdateSessionDto): Promise<SessionHeader> {
  const updated = await sessionService.updateSession(id, dto);
  project$.sessions.set((prev) =>
    prev.map((sess) => (sess.id === id ? { ...sess, ...updated } : sess)),
  );
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
  return updated;
}

export async function deleteSession(id: string): Promise<void> {
  await sessionService.deleteSession(id);
  project$.sessions.set((prev) => prev.filter((sess) => sess.id !== id));
  clearStatus(id);
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
}

export async function restoreSession(id: string): Promise<void> {
  await sessionService.restoreSession(id);
  const restored = project$.deletedSessions.peek().find((sess) => sess.id === id);
  project$.deletedSessions.set((prev) => prev.filter((sess) => sess.id !== id));
  if (restored) {
    project$.sessions.unshift(restored);
  }
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
}

export async function permanentlyDeleteSession(id: string): Promise<void> {
  await sessionService.permanentlyDeleteSession(id);
  project$.deletedSessions.set((prev) => prev.filter((sess) => sess.id !== id));
  queryClient.invalidateQueries({ queryKey: sessionKeys.all }).catch(() => {});
}

/** Re-fetch a session header from the backend and patch it in-place (e.g. after an auto-renamed title). */
export async function refreshSessionHeader(sessionId: string): Promise<void> {
  try {
    const detail = await sessionService.getSession(sessionId);
    const header = detail.header;
    project$.sessions.set((prev) =>
      prev.map((sess) => (sess.id === sessionId ? { ...sess, ...header } : sess)),
    );
    if (header.status) setStatus(sessionId, header.status);
  } catch {
    // Ignore refresh failures — the header will update on next load.
  }
}
