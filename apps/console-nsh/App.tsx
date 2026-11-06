// Console — the NSH app's shell. Phase 2.1: the title bar and the
// project/session sidebar read real data from the Go server through
// console-core. Panes arrive in subphase 2.3.

import { mount, signal, Text, View } from "nsh";
import {
  Console,
  type ProjectSummary,
  type SessionSummary,
} from "console-nsh/native";
import { Sidebar } from "./src/layout/sidebar";
import { TitleBar } from "./src/layout/title_bar";

// The Go dev server's default port; the desktop app defaults the same way.
const backendUrl = "http://localhost:2003";

const loading = signal(true);
const projects = signal<ProjectSummary[]>([]);
const selectedProjectId = signal<string | null>(null);
const sessionsLoading = signal(false);
const sessions = signal<SessionSummary[]>([]);
const selectedSessionId = signal<string | null>(null);

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
          onProject={selectProject}
          onSession={(id) => selectedSessionId.set(id)}
        />
        <View class="flex flex-1 flex-col items-center justify-center p-6">
          <Text class="text-sm text-muted-foreground">
            Placeholder — panes arrive in subphase 2.3.
          </Text>
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
