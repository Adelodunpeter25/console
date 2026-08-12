import React from "react";
import type { SessionHeader, SessionStatus } from "@console/types";
import { FolderClosed, Trash2 } from "lucide-react";
import { useAppStore } from "../../store/useAppStore";
import { useProjectStore } from "../../store/useProjectStore";
import { useSessionStatusStore } from "../../store/useSessionStatusStore";
import { useWorkspaceStore } from "../../layout/useWorkspaceStore";
import { useContextMenu } from "../common/ContextMenu";
import { formatRelativeTime } from "../../utils/time";
import { basename } from "../../utils/format";

const STATUS_DOT: Record<SessionStatus, string> = {
  idle: "bg-foreground-muted",
  working: "bg-blue-500",
  done: "bg-green-500",
  needs_attention: "bg-amber-500",
};

interface SessionItemProps {
  session: SessionHeader;
  isActive: boolean;
}

/**
 * Single session row in the sidebar — status dot, title, working folder,
 * timestamp, and hover Delete button.
 */
export const SessionItem = React.memo(function SessionItem({
  session,
  isActive,
}: SessionItemProps) {
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const deleteSession = useProjectStore((state) => state.deleteSession);
  const projects = useProjectStore((state) => state.projects);
  const openChatTab = useWorkspaceStore((state) => state.openChatTab);
  const closeChatTab = useWorkspaceStore((state) => state.closeChatTab);
  const contextMenu = useContextMenu();
  const liveStatus = useSessionStatusStore((state) => state.statuses[session.id]);
  const status: SessionStatus = liveStatus ?? session.status ?? "idle";
  const projectId = React.useMemo(
    () =>
      session.projectId ?? projects.find((project) => project.path === session.cwd)?.id ?? null,
    [session.projectId, session.cwd, projects],
  );

  const handleOpen = () => {
    if (!projectId) return;
    setSelectedProjectId(projectId);
    openChatTab({
      type: "chat",
      projectId,
      sessionId: session.id,
      title: session.title || "Untitled Chat",
    });
  };

  const handleContextMenu = (event: React.MouseEvent) => {
    event.preventDefault();
    contextMenu.open(event.clientX, event.clientY, [
      { label: "Rename", onClick: () => {} },
      {
        label: "Delete",
        danger: true,
        separatorBefore: true,
        onClick: () => {},
      },
    ]);
  };

  return (
    <div
      className={`group relative flex flex-col gap-0.5 px-2.5 py-1.5 rounded-lg cursor-pointer transition-colors ${
        isActive
          ? "bg-white/[0.08] text-foreground"
          : "text-foreground-secondary hover:bg-white/[0.04] hover:text-foreground"
      }`}
      onClick={handleOpen}
      onContextMenu={handleContextMenu}
    >
      {/* Row 1: Fixed Status Dot Container + Title + Right Action Slot */}
      <div className="flex items-center gap-2 min-w-0">
        <div className="w-4 h-4 shrink-0 flex items-center justify-center">
          {status === "working" ? (
            <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
          ) : (
            <div
              className={`w-2 h-2 rounded-full ${STATUS_DOT[status] ?? STATUS_DOT.idle}`}
            />
          )}
        </div>

        <span className={`text-xs truncate flex-1 min-w-0 ${isActive ? "font-semibold text-foreground" : ""}`}>
          {session.title || "Untitled Chat"}
        </span>

        {/* Right side: Time by default, replaced by Delete button on group hover */}
        <div className="shrink-0 flex items-center justify-end w-9 text-right">
          <span className="text-[11px] text-foreground-muted group-hover:hidden transition-opacity">
            {formatRelativeTime(session.updatedAt, true)}
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              deleteSession(session.id);
              if (projectId) closeChatTab(projectId, session.id);
            }}
            className="hidden group-hover:flex items-center justify-center p-1 rounded hover:bg-white/10 text-foreground-muted hover:text-danger transition-colors cursor-pointer"
            title="Delete session"
            aria-label={`Delete ${session.title || "session"}`}
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>

      {/* Row 2: Working Folder path aligned flush under Title text (24px left indent) */}
      {session.cwd && (
        <div className="flex items-center gap-1.5 pl-6 min-w-0">
          <FolderClosed size={11} className="text-foreground-muted shrink-0" />
          <span className="text-[11px] text-foreground-muted truncate min-w-0">
            {basename(session.cwd)}
          </span>
        </div>
      )}
    </div>
  );
});
