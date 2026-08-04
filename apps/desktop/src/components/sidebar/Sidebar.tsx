import React from "react";
import { FolderOpen, Plus, SquarePen } from "lucide-react";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { useAppStore, useProjectStore } from "../../store";
import { SessionItem } from "./SessionItem";
import { dayBucket, formatDayGroup } from "../../utils/time";
import type { SessionHeader } from "@console/types";

/** Group sessions into labeled date buckets, newest-first by last-updated. */
function groupSessionsByDate(sessions: SessionHeader[]): Array<{ label: string; items: SessionHeader[] }> {
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

/**
 * Left sidebar — flat session list with New Chat action.
 * Rendered inside a ResizablePanel by ChatPage; width is passed in so the
 * internal container matches the panel (default 288px = w-72).
 */
export function Sidebar({ width = 288 }: { width?: number }) {
  const navigate = useNavigate();
  const { sidebarOpen, selectedSessionId, setSelectedProjectId, setSelectedSessionId } = useAppStore();
  const { projects, loading, loadProjects, sessions, sessionsLoading, loadSessions, createSession } =
    useProjectStore();

  React.useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  React.useEffect(() => {
    loadSessions();
  }, [loadSessions]);

  if (!sidebarOpen) return null;

  const handleGlobalNewChat = async () => {
    const targetProject = projects[0];
    if (!targetProject) {
      toast.error("Please add a project first.");
      return;
    }
    const session = await createSession(targetProject.path, targetProject.id, "New Chat");
    setSelectedProjectId(targetProject.id);
    setSelectedSessionId(session.id);
  };

  return (
    <div className="bg-sidebar border-r border-border flex flex-col h-full shrink-0 select-none" style={{ width }}>
      {/* Top Actions Bar */}
      <div className="px-3 pt-3 pb-2 shrink-0">
        <button
          onClick={handleGlobalNewChat}
          className="w-full flex items-center gap-2 px-3 py-2 rounded-lg bg-white/[0.06] text-foreground-secondary hover:bg-white/[0.1] hover:text-foreground transition-colors cursor-pointer"
          title="New Chat"
        >
          <SquarePen size={15} />
          <span className="text-xs font-medium">New chat</span>
        </button>
      </div>

      {/* Category Header */}
      <div className="px-4 pt-2 pb-1">
        <span className="text-[11px] font-bold tracking-wider text-foreground-muted uppercase">
          CHATS
        </span>
      </div>

      {/* Session List */}
      <div className="flex-1 overflow-y-auto px-2 py-1 space-y-0.5">
        {loading || sessionsLoading ? (
          <div className="flex items-center justify-center py-8">
            <span className="text-xs text-foreground-muted">Loading...</span>
          </div>
        ) : sessions.length === 0 ? (
          <div className="px-2 py-8 text-center">
            <FolderOpen size={24} className="mx-auto text-foreground-muted mb-2" />
            <p className="text-xs text-foreground-muted">No chats yet.</p>
          </div>
        ) : (
          groupSessionsByDate(sessions).map((group) => (
            <div key={group.label} className="space-y-0.5">
              <div className="px-2 pt-3 pb-1">
                <span className="text-[10px] font-bold tracking-wider text-foreground-muted uppercase">
                  {group.label}
                </span>
              </div>
              {group.items.map((session) => (
                <SessionItem
                  key={session.id}
                  session={session}
                  isActive={session.id === selectedSessionId}
                />
              ))}
            </div>
          ))
        )}
      </div>

      {/* Bottom Footer Bar — New Project opens the settings page */}
      <div className="border-t border-border/80 p-3 flex items-center justify-between shrink-0">
        <button
          onClick={() => navigate({ to: "/settings" })}
          className="flex items-center gap-2 text-xs font-medium text-foreground-secondary hover:text-foreground transition-colors"
        >
          <Plus size={14} />
          <span>New Project</span>
        </button>
      </div>
    </div>
  );
}
