import React from "react";
import { FolderOpen, FolderPlus, Search, Settings, SquarePen } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useWorkspaceStore } from "../../layout/useWorkspaceStore";
import { useVirtualList } from "../../hooks/useVirtualList";
import { SessionItem } from "./SessionItem";
import { dayBucket, formatDayGroup } from "../../utils/time";
import { tauriApi } from "../../lib/tauri-api";
import type { SessionHeader } from "@console/types";

/** Group sessions into labeled date buckets, newest-first by last-updated. */
function groupSessionsByDate(
  sessions: SessionHeader[],
): Array<{ label: string; items: SessionHeader[] }> {
  // The server already returns sessions ordered by updated_at DESC, so group
  // the incoming order directly — a session touched today surfaces under Today
  // even if it was created earlier.
  const buckets = new Map<number, SessionHeader[]>();
  for (const session of sessions) {
    const bucket = dayBucket(session.updatedAt);
    const list = buckets.get(bucket) ?? [];
    list.push(session);
    buckets.set(bucket, list);
  }
  // Sort buckets by recency (today first), sessions stay newest-first.
  return Array.from(buckets.entries())
    .sort((a, b) => a[0] - b[0])
    .map(([bucket, items]) => ({ label: formatDayGroup(bucket), items }));
}

/** Flatten grouped sessions into a single list with sticky group headers
 *  interleaved as sentinel items, so the virtualizer can render them. */
type FlatEntry =
  | { kind: "header"; label: string; key: string }
  | { kind: "session"; session: SessionHeader; key: string };

function flattenGroups(
  groups: Array<{ label: string; items: SessionHeader[] }>,
): FlatEntry[] {
  const result: FlatEntry[] = [];
  for (const group of groups) {
    result.push({ kind: "header", label: group.label, key: `header-${group.label}` });
    for (const session of group.items) {
      result.push({ kind: "session", session, key: session.id });
    }
  }
  return result;
}

/**
 * Left sidebar — flat session list with New Chat, Search, and Add Project actions.
 * Rendered inside a ResizablePanel by ChatPage; width is passed in so the
 * internal container matches the panel (default 288px = w-72).
 */
export function Sidebar({ width = 288 }: { width?: number }) {
  const navigate = useNavigate();
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const setCommandPaletteOpen = useAppStore((state) => state.setCommandPaletteOpen);
  const openChatTab = useWorkspaceStore((state) => state.openChatTab);
  const projects = useProjectStore((state) => state.projects);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const addProject = useProjectStore((state) => state.addProject);
  const sessions = useProjectStore((state) => state.sessions);
  const sessionsLoading = useProjectStore((state) => state.sessionsLoading);
  const loadSessions = useProjectStore((state) => state.loadSessions);
  const createSession = useProjectStore((state) => state.createSession);

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  React.useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  const flatEntries = React.useMemo(() => {
    const groups = groupSessionsByDate(sessions);
    return flattenGroups(groups);
  }, [sessions]);

  const getItemSize = React.useCallback(
    (index: number) => {
      const entry = flatEntries[index];
      if (entry?.kind === "header") return 38;
      return 52;
    },
    [flatEntries],
  );

  const { parentRef, virtualItems, totalSize } = useVirtualList({
    items: flatEntries,
    getItemSize,
    overscan: 8,
  });

  if (useAppStore.getState().sidebarOpen === false) return null;

  const handleGlobalNewChat = async () => {
    const session = await createSession("", "", "New Chat");
    setSelectedProjectId(null);
    openChatTab({
      type: "chat",
      projectId: "",
      sessionId: session.id,
      title: session.title,
    });
  };

  const handleAddProject = async () => {
    try {
      const result = await tauriApi.pickFolder();
      if (result && result.path) {
        await addProject(result.path);
      }
    } catch {
      // User cancelled picker
    }
  };

  return (
    <div
      className="bg-sidebar border-r border-border flex flex-col h-full shrink-0 select-none"
      style={{ width }}
    >
      {/* Top Actions Bar */}
      <div className="px-3 pt-3 pb-1 shrink-0 space-y-1">
        <button
          onClick={handleGlobalNewChat}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-foreground-secondary hover:bg-white/[0.06] hover:text-foreground transition-colors cursor-pointer"
          title="New Chat"
        >
          <SquarePen size={15} />
          <span className="text-xs font-medium">New chat</span>
        </button>

        <button
          onClick={() => setCommandPaletteOpen(true)}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-foreground-secondary hover:bg-white/[0.06] hover:text-foreground transition-colors cursor-pointer"
          title="Search (Command Palette)"
        >
          <Search size={15} />
          <span className="text-xs font-medium">Search</span>
        </button>
      </div>

      {/* Session List */}
      <div ref={parentRef} className="flex-1 overflow-y-auto px-2 py-1 space-y-0.5">
        {sessionsLoading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-xs text-foreground-muted">Loading...</span>
          </div>
        ) : sessions.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <FolderOpen size={24} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">No chats yet.</p>
          </div>
        ) : (
          <div
            style={{ height: totalSize, position: "relative", width: "100%" }}
          >
            {virtualItems.map((virtualRow) => {
              const entry = flatEntries[virtualRow.index];
              if (!entry) return null;

              if (entry.kind === "header") {
                const isFirstHeader = virtualRow.index === 0;
                return (
                  <div
                    key={entry.key}
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                    className="px-2 pt-3.5 pb-1 flex items-center justify-between min-h-[38px]"
                  >
                    <span className="text-[10px] font-bold tracking-wider text-foreground-muted uppercase">
                      {entry.label}
                    </span>
                    {isFirstHeader && (
                      <button
                        onClick={handleAddProject}
                        className="p-1 rounded text-foreground-muted hover:text-foreground hover:bg-white/10 transition-colors cursor-pointer"
                        title="Add Project Folder to Database"
                        aria-label="Add Project Folder"
                      >
                        <FolderPlus size={14} />
                      </button>
                    )}
                  </div>
                );
              }

              return (
                <div
                  key={entry.key}
                  style={{
                    position: "absolute",
                    top: 0,
                    left: 0,
                    width: "100%",
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <SessionItem
                    session={entry.session}
                    isActive={entry.session.id === selectedSessionId}
                  />
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Bottom Footer Bar — Settings opens the settings page */}
      <div className="border-t border-border/80 p-3 flex items-center justify-between shrink-0">
        <button
          onClick={() => navigate({ to: "/settings" })}
          className="flex items-center gap-2 text-xs font-medium text-foreground-secondary hover:text-foreground transition-colors"
        >
          <Settings size={14} />
          <span>Settings</span>
        </button>
      </div>
    </div>
  );
}
