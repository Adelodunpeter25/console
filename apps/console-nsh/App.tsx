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
import { PairingCard } from "./src/layout/pairing_card";
import { createWorkspaceStore } from "./src/workspace/workspace_store";
import { WorkspaceView, chatController } from "./src/workspace/workspace_view";
import { WorkspaceShortcuts } from "./src/workspace/shortcuts";
import type { GitBranchInfo } from "./src/common/workspace_footer";
import type { WorkspaceNode } from "./src/workspace/model";
import type { SortMode } from "./src/layout/sidebar/group_header";
import { GitService, SessionService } from "./src/services";

// The Go dev server's default port; the desktop app defaults the same way.
// A caller may point the loaders at another base URL (tests, discovery).
export const backendUrl = "http://localhost:2003";

const loading = signal(true);
const projects = signal<ProjectSummary[]>([]);
const projectsError = signal<string | null>(null);
const selectedProjectId = signal<string | null>(null);
const projectBranches = signal<GitBranchInfo[]>([]);
const sessionsLoading = signal(false);
const sessions = signal<SessionSummary[]>([]);
const sessionsError = signal<string | null>(null);
const selectedSessionId = signal<string | null>(null);
const sortMode = signal<SortMode>("date");

// The launcher composer (Cmd+N) and the workspace pane tree.
const launcherOpen = signal(false);
const store = createWorkspaceStore();

// Pairing gate: the shell stays on the pairing card until the app holds a
// device credential for the backend (the desktop's startup check).
const paired = signal(true);

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
  const p = projects().find((item) => item.id === id);
  if (p?.path) {
    void (async () => {
      const branches = await GitService.listBranches(backendUrl, p.path);
      projectBranches.set(branches);
    })();
  } else {
    projectBranches.set([]);
  }
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

// Deferred off the reactive pass: nsh's native bridge must not be called
// while components run (the save effect below does the same). Mounting
// first also means the reactive runtime exists when the bridge call lands.
const startup = () => {
  void (async () => {
    try {
      const saved = await Console.loadWorkspaceLayout(WORKSPACE_KEY);
      if (saved != null) store.restoreTree(JSON.parse(saved) as WorkspaceNode);
      else store.openTab(null, WELCOME_TAB);
    } catch {
      store.openTab(null, WELCOME_TAB);
    }
  })();
  // Startup pairing check: an unpaired app boots onto the pairing card and
  // only loads data once paired (the desktop's bootstrap does the same).
  paired.set(Console.authPaired(backendUrl));
  if (paired()) {
    void loadProjects();
    void loadSessions(null);
  }
};
setTimeout(startup, 0);

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
      {paired() ? (
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
          onToggleSettle={(id) => {
            void (async () => {
              try {
                const s = sessions().find((item) => item.id === id);
                if (s) {
                  await SessionService.toggleSettle(backendUrl, s);
                  await loadSessions(selectedProjectId());
                }
              } catch (err) {
                console.error("toggle settle failed:", errorMessage(err));
              }
            })();
          }}
          onDeleteSession={(id) => {
            void (async () => {
              try {
                await SessionService.delete(backendUrl, id);
                await loadSessions(selectedProjectId());
              } catch (err) {
                console.error("delete session failed:", errorMessage(err));
              }
            })();
          }}
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
              branches={projectBranches()}
              onLaunch={(draft) => {
                launcherOpen.set(false);
                launcherDraft.set("");
                void (async () => {
                  try {
                    // The launcher's submit creates the session, then opens
                    // its chat tab and streams the draft into it (the
                    // desktop's floating composer does the same through
                    // console-core).
                    const header = await SessionService.create(
                      backendUrl,
                      selectedProjectId() ?? "",
                      null,
                    );
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
      ) : (
        <PairingCard baseUrl={backendUrl} onPaired={() => {
          paired.set(true);
          void loadProjects();
          void loadSessions(null);
        }} />
      )}
    </View>
  );
}

const launcherDraft = signal("");

// Startup pairing check and the welcome tab land in `startup`, deferred
// until after mount (the bridge must not be called while components run).

mount(App);

export {
  App,
  paired,
  loadProjects,
  loadSessions,
  selectProject,
  projectsError,
  sessionsError,
  launcherOpen,
  store,
};
