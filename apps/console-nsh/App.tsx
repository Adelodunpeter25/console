// Console — the NSH app's shell. Phase 2.1: the title bar and the
// project/session sidebar read real data from the Go server through
// console-core. Panes arrive in subphase 2.3.

import { mount, signal, Text, View } from "nsh";
import {
  Console,
  type ProjectSummary,
  type SessionSummary,
} from "console-nsh/native";
import { Sidebar } from "./src/layout/sidebar/sidebar";
import { TitleBar } from "./src/layout/title_bar";
import { EmptyChatState } from "./src/common/empty_state";
import type { SortMode } from "./src/layout/sidebar/group_header";
import type { DraftSummary } from "./src/layout/sidebar/draft_item";

// The Go dev server's default port; the desktop app defaults the same way.
const backendUrl = "http://localhost:2003";

const loading = signal(true);
const projects = signal<ProjectSummary[]>([]);
const selectedProjectId = signal<string | null>(null);
const sessionsLoading = signal(false);
const sessions = signal<SessionSummary[]>([]);
const selectedSessionId = signal<string | null>(null);
const sortMode = signal<SortMode>("date");
// Frontend-only until the drafts service moves over (Rust pass).
const drafts = signal<DraftSummary[]>([]);

async function loadProjects() {
  loading.set(true);
  projects.set(await Console.projects(backendUrl));
  loading.set(false);
}

async function loadSessions(projectId: string | null) {
  sessionsLoading.set(true);
  sessions.set(await Console.sessions(backendUrl, projectId ?? ""));
  sessionsLoading.set(false);
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
          selectedProjectId={selectedProjectId()}
          sessions={sessions()}
          sessionsLoading={sessionsLoading()}
          drafts={drafts()}
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

export { App, loadProjects, loadSessions, selectProject };
