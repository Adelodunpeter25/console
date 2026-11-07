// The sessions store: the sidebar's shared data layer — projects, the
// selected project's sessions, and their loading/error state — behind one
// domain object, so the shell's components read signals and call loaders
// instead of touching the backend themselves (the desktop's `AppState`
// equivalent for these two collections).

import { signal } from "nsh";
import type { ProjectSummary, SessionSummary } from "console-nsh/native";
import { backend } from "../services/backend";

export function createSessionsStore(baseUrl: string) {
  const loading = signal(true);
  const projects = signal<ProjectSummary[]>([]);
  const projectsError = signal<string | null>(null);
  const selectedProjectId = signal<string | null>(null);
  const sessionsLoading = signal(false);
  const sessions = signal<SessionSummary[]>([]);
  const sessionsError = signal<string | null>(null);

  function errorMessage(err: unknown): string {
    return err instanceof Error ? err.message : String(err);
  }

  async function loadProjects(): Promise<void> {
    loading.set(true);
    try {
      projects.set(await backend.projects(baseUrl));
      projectsError.set(null);
    } catch (err) {
      projects.set([]);
      projectsError.set(errorMessage(err));
    } finally {
      loading.set(false);
    }
  }

  async function loadSessions(projectId: string | null): Promise<void> {
    sessionsLoading.set(true);
    try {
      sessions.set(await backend.sessions(baseUrl, projectId ?? ""));
      sessionsError.set(null);
    } catch (err) {
      sessions.set([]);
      sessionsError.set(errorMessage(err));
    } finally {
      sessionsLoading.set(false);
    }
  }

  function selectProject(id: string): void {
    selectedProjectId.set(id);
    void loadSessions(id);
  }

  function session(id: string): SessionSummary | undefined {
    return sessions().find((s) => s.id === id);
  }

  return {
    loading,
    projects,
    projectsError,
    selectedProjectId,
    sessionsLoading,
    sessions,
    sessionsError,
    loadProjects,
    loadSessions,
    selectProject,
    session,
  };
}

export type SessionsStore = ReturnType<typeof createSessionsStore>;
