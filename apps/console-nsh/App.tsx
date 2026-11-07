// Console — the NSH app's shell. Phase 2.1 (title bar, sidebar with project
// groups and drafts) beside phase 2.3 (the workspace pane tree: tabs,
// splits, persisted layouts, the Cmd+N launcher composer and the global
// shortcuts).

import { effect, mount, signal, View } from "nsh";
import {
  Console,
  type ProjectSummary,
  type SessionSummary,
} from "console-nsh/native";
import { FloatingComposer } from "./src/common/floating_composer";
import { Sidebar } from "./src/layout/sidebar/sidebar";
import { TitleBar } from "./src/layout/title_bar";
import { createWorkspaceStore } from "./src/workspace/workspace_store";
import { WorkspaceView, chatController } from "./src/workspace/workspace_view";
import { WorkspaceShortcuts } from "./src/workspace/shortcuts";
import type { WorkspaceNode } from "./src/workspace/model";
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

// The launcher composer (Cmd+N) and the workspace pane tree.
const launcherOpen = signal(false);
const store = createWorkspaceStore();

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

/// Opens a session's chat tab in the workspace (the sidebar and the
/// launcher both land here). Real transcript wiring arrives with the chat
/// pane content (2.4); the tab opens now so the shell is usable.
function openChat(sessionId: string, title: string) {
  store.openTab(null, { type: "chat", sessionId, title });
  selectedSessionId.set(sessionId);
}

function selectSession(id: string) {
  const session = sessions().find((s) => s.id === id);
  openChat(id, session?.title ?? id);
}

// Workspace persistence: restore the saved layout once, then write the
// whole document on every revision (the document is small — a pane tree —
// so whole-document writes stay cheap). A restored tree carries no session
// content yet; its chat tabs re-render when the session wiring lands (2.4).
const WORKSPACE_KEY = "default";

const WELCOME_TAB = {
  type: "chat" as const,
  sessionId: "welcome",
  title: "Welcome",
};

void (async () => {
  try {
    const saved = await Console.loadWorkspaceLayout(WORKSPACE_KEY);
    if (saved != null) store.restoreTree(JSON.parse(saved) as WorkspaceNode);
    else store.openTab(null, WELCOME_TAB);
  } catch {
    store.openTab(null, WELCOME_TAB);
  }
})();

effect(() => {
  if (store.revision() === 0) return;
  // Deferred off the reactive pass: nsh's native bridge rejects calls made
  // from a running component's engine thread, and a short debounce also
  // collapses bursts of revisions (a streaming open bumps several) into one
  // write.
  setTimeout(() => {
    // Fire-and-forget: a failed write (disk full, host gone) leaves the
    // tree in memory; the next successful write catches up.
    Console.saveWorkspaceLayout(WORKSPACE_KEY, JSON.stringify(store.tree()));
  }, 250);
});

function App() {
  return (
    <View class="flex flex-1 flex-col bg-background">
      <WorkspaceShortcuts store={store} onNewChat={() => launcherOpen.set(true)} />
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
          onSession={selectSession}
          onSortMode={(mode) => sortMode.set(mode)}
          onNewChat={() => launcherOpen.set(true)}
        />
        <View class="flex flex-1 flex-col gap-2 p-2">
          <View class="flex flex-1 flex-col border border-border rounded-lg overflow-hidden">
            <WorkspaceView store={store} baseUrl={backendUrl} />
          </View>
          {launcherOpen() ? (
            <FloatingComposer
              value={launcherDraft()}
              onChange={(text: string) => launcherDraft.set(text)}
              onSend={() => {}}
              projects={projects().map((p) => ({ id: p.id, name: p.name, path: p.path ?? "" }))}
              onLaunch={(draft) => {
                launcherOpen.set(false);
                launcherDraft.set("");
                void (async () => {
                  try {
                    // The launcher's submit creates the session, then opens
                    // its chat tab and streams the draft into it (the
                    // desktop's floating composer does the same through
                    // console-core).
                    const header = JSON.parse(
                      await Console.createSession(backendUrl, selectedProjectId() ?? "", null),
                    ) as { id: string };
                    if (draft.trim() !== "") {
                      void chatController(backendUrl, header.id).send(draft);
                    }
                    openChat(header.id, draft.slice(0, 48) || "New chat");
                  } catch (err) {
                    // Session creation failed (backend down): fall back to a
                    // placeholder tab so the draft text is not lost.
                    openChat(`draft:${Date.now()}`, draft.slice(0, 48) || "New chat");
                    console.error("session create failed:", errorMessage(err));
                  }
                })();
              }}
            />
          ) : null}
        </View>
      </View>
    </View>
  );
}

const launcherDraft = signal("");

void (async () => {
  await loadProjects();
  await loadSessions(null);
})();

mount(App);

export {
  App,
  loadProjects,
  loadSessions,
  selectProject,
  projectsError,
  sessionsError,
  launcherOpen,
  store,
};
