import React from "react";
import { useSessions, useCreateSession, useDeleteSession } from "@console/api";
import { activeProjectId$, activeSessionId$ } from "../../state/index.js";
import { observer } from "@legendapp/state/react";
import { Folder, SquarePen, MessageSquare, Trash2 } from "lucide-react";
import type { ProjectInfo } from "@console/types";
import { formatRelativeTime } from "../../utils/index.js";

export interface SidebarListItemProps {
  project: ProjectInfo;
  isExpanded: boolean;
  onToggleExpand: () => void;
}

export const SidebarListItem = observer(
  ({ project, isExpanded, onToggleExpand }: SidebarListItemProps) => {
    const activeSessionId = activeSessionId$.get();
    const activeProjectId = activeProjectId$.get();
    const createSessionMutation = useCreateSession();
    const deleteSessionMutation = useDeleteSession();

    const { data: sessions = [] } = useSessions({ projectId: project.id });
    const isSelectedProject = activeProjectId === project.id;

    const handleCreateNewSession = async (e: React.MouseEvent) => {
      e.stopPropagation();
      try {
        const created = await createSessionMutation.mutateAsync({
          cwd: project.path,
          projectId: project.id,
          title: "New Session",
        });
        activeProjectId$.set(project.id);
        activeSessionId$.set(created.id);
      } catch (err) {
        alert(err instanceof Error ? err.message : "Failed to create new session");
      }
    };

    const handleDeleteSession = async (sessionId: string, e: React.MouseEvent) => {
      e.stopPropagation();
      if (!confirm("Delete session?")) return;
      try {
        await deleteSessionMutation.mutateAsync(sessionId);
        if (activeSessionId === sessionId) {
          activeSessionId$.set(null);
        }
      } catch (err) {
        alert(err instanceof Error ? err.message : "Failed to delete session");
      }
    };

    return (
      <div className="flex flex-col select-none">
        {/* Project Header Row - Clean layout without caret button, matching screenshot */}
        <div
          onClick={() => {
            activeProjectId$.set(project.id);
            onToggleExpand();
          }}
          className={`group px-2 py-1.5 rounded-md text-xs flex items-center justify-between cursor-pointer transition-colors ${
            isSelectedProject
              ? "bg-accent/80 text-foreground font-medium"
              : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
          }`}
        >
          <div className="flex items-center gap-2.5 overflow-hidden">
            <Folder size={15} className="text-muted-foreground group-hover:text-foreground shrink-0 transition-colors" />
            <span className="truncate text-xs font-medium tracking-tight text-foreground/90">{project.name}</span>
          </div>

          <button
            onClick={handleCreateNewSession}
            title="New Chat Session"
            className="opacity-0 group-hover:opacity-100 p-1 hover:bg-card hover:text-foreground text-muted-foreground rounded transition-opacity cursor-pointer"
          >
            <SquarePen size={13} />
          </button>
        </div>

        {/* Child Sessions List */}
        {isExpanded && (
          <div className="ml-4 mt-0.5 pl-2 border-l border-border/30 flex flex-col gap-0.5">
            {sessions.length === 0 ? (
              <div className="px-2 py-1 text-[11px] text-muted-foreground/60 italic">
                No sessions yet
              </div>
            ) : (
              sessions.map((sess) => {
                const isActiveSession = activeSessionId === sess.id;
                const timeAgo = formatRelativeTime(sess.createdAt || sess.updatedAt);
                return (
                  <div
                    key={sess.id}
                    onClick={() => {
                      activeProjectId$.set(project.id);
                      activeSessionId$.set(sess.id);
                    }}
                    className={`group px-2 py-1 rounded text-[11px] flex items-center justify-between cursor-pointer transition-colors ${
                      isActiveSession
                        ? "bg-primary/10 text-primary font-medium"
                        : "text-muted-foreground hover:bg-accent/30 hover:text-foreground"
                    }`}
                  >
                    <div className="flex items-center gap-2 truncate">
                      <MessageSquare size={12} className={isActiveSession ? "text-primary shrink-0" : "text-muted-foreground shrink-0"} />
                      <span className="truncate">{sess.title || "New Chat"}</span>
                    </div>

                    <div className="flex items-center gap-1.5 shrink-0">
                      {timeAgo && (
                        <span className="text-[10px] text-muted-foreground/60 font-mono opacity-80 group-hover:opacity-100">
                          {timeAgo}
                        </span>
                      )}
                      <button
                        onClick={(e) => handleDeleteSession(sess.id, e)}
                        className="opacity-0 group-hover:opacity-100 p-0.5 hover:text-destructive transition-opacity"
                      >
                        <Trash2 size={11} />
                      </button>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
    );
  }
);
