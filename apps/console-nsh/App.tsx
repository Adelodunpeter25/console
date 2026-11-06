// Console — the NSH app's shell. Phase 2.1: the title bar and the
// project/session sidebar read real data from the Go server through
// console-core. Panes arrive in subphase 2.3.

import { mount, signal, View } from "nsh";
import {
  Console,
  type ProjectSummary,
  type SessionSummary,
} from "console-nsh/native";
import { Sidebar } from "./src/layout/sidebar/sidebar";
import { TitleBar } from "./src/layout/title_bar";
import { EmptyChatState } from "./src/common/empty_state";
import type { SortMode } from "./src/layout/sidebar/group_header";

// The Go dev server's default port; the desktop app defaults the same way.
// A caller may point the loaders at another base URL (tests, discovery).
export const backendUrl = "http://localhost:2003";

const loading = signal(true);
const projects = signal<ProjectSummary[]>([]);
const projectsError = signal<string | null>(null);
const selectedProjectId = signal<string | null>(null);
const sessionsLoading = signal(false);
const sessions = signal<SessionSummary[]>([]);
const sessionsError = signal<string | null>(null);
const selectedSessionId = signal<string | null>(null);
const sortMode = signal<SortMode>("date");

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

async function loadProjects(baseUrl: string = backendUrl) {
  loading.set(true);
  try {
    projects.set(await Console.projects(baseUrl));
    projectsError.set(null);
  } catch (err) {
    projects.set([]);
    projectsError.set(errorMessage(err));
  } finally {
    loading.set(false);
  }
}

async function loadSessions(projectId: string | null, baseUrl: string = backendUrl) {
  sessionsLoading.set(true);
  try {
    sessions.set(await Console.sessions(baseUrl, projectId ?? ""));
    sessionsError.set(null);
  } catch (err) {
    sessions.set([]);
    sessionsError.set(errorMessage(err));
  } finally {
    sessionsLoading.set(false);
  }
}

function selectProject(id: string) {
  selectedProjectId.set(id);
  selectedSessionId.set(null);
  void loadSessions(id);
}

function App() {
  return (
    <View class="flex flex-1 flex-col bg-background">
      <TitleBar />
      <View class="flex flex-1 flex-row">
        <Sidebar
          loading={loading()}
          projects={projects()}
          projectsError={projectsError()}
          selectedProjectId={selectedProjectId()}
          sessions={sessions()}
          sessionsLoading={sessionsLoading()}
          sessionsError={sessionsError()}
          activeSessionId={selectedSessionId()}
          sortMode={sortMode()}
          onProject={selectProject}
          onSession={(id) => selectedSessionId.set(id)}
          onSortMode={(mode) => sortMode.set(mode)}
          onNewChat={() => selectedSessionId.set(null)}
        />
        <View class="flex flex-1 flex-col">
          <EmptyChatState onNewChat={() => selectedSessionId.set(null)} />
        </View>
      </View>
    </View>
  );
}

void (async () => {
  await loadProjects();
  await loadSessions(null);
})();

mount(App);

export { App, loadProjects, loadSessions, selectProject, projectsError, sessionsError };
