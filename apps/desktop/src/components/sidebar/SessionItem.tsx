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
import { api } from "../../lib/api";

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
 * timestamp on second row, and hover Delete button with Tauri confirm dialog for non-done chats.
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

  const handleDragStart = (e: React.DragEvent) => {
    const pid = projectId ?? session.projectId ?? "";
    const config = {
      type: "chat" as const,
      projectId: pid,
      sessionId: session.id,
      title: session.title || "Untitled Chat",
    };
    useWorkspaceStore.getState().setDraggedTab({ tabConfig: config });
    const payload = JSON.stringify({ tabConfig: config });
    e.dataTransfer.setData("application/json", payload);
    e.dataTransfer.setData("text/plain", payload);
    e.dataTransfer.effectAllowed = "move";
  };

  const handleDragEnd = () => {
    useWorkspaceStore.getState().setDraggedTab(null);
  };

  const handleOpen = () => {
    const pid = projectId ?? session.projectId ?? "";
    if (pid) {
      setSelectedProjectId(pid);
    }
    openChatTab({
      type: "chat",
      projectId: pid,
      sessionId: session.id,
      title: session.title || "Untitled Chat",
    });
  };

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (status !== "done") {
      const confirmed = await api.confirmDialog(
        "Delete Active Session",
        `"${session.title || "Untitled Chat"}" is currently not marked as done. Are you sure you want to delete it?`,
      );
      if (!confirmed) return;
    }
    deleteSession(session.id);
    closeChatTab(session.id);
  };

  const handleContextMenu = (event: React.MouseEvent) => {
    event.preventDefault();
    contextMenu.open(event.clientX, event.clientY, [
      { label: "Rename", onClick: () => {} },
      {
        label: "Delete",
        danger: true,
        separatorBefore: true,
        onClick: () => {
          void (async () => {
            if (status !== "done") {
              const confirmed = await api.confirmDialog(
                "Delete Active Session",
                `"${session.title || "Untitled Chat"}" is currently not marked as done. Are you sure you want to delete it?`,
              );
              if (!confirmed) return;
            }
            deleteSession(session.id);
            closeChatTab(session.id);
          })();
        },
      },
    ]);
  };

  return (
    <div
      draggable
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      style={{ WebkitUserDrag: "element" } as React.CSSProperties}
      className={`group relative flex flex-col justify-between py-2 px-3 min-h-[48px] rounded-lg cursor-grab active:cursor-grabbing transition-colors ${
        isActive
          ? "bg-white/[0.08] text-foreground"
          : "text-foreground-secondary hover:text-foreground"
      }`}
      onClick={handleOpen}
      onContextMenu={handleContextMenu}
    >
      {/* Row 1: Fixed Status Container + Title + Right-aligned Hover Delete Button */}
      <div className="flex items-center gap-1.5 min-w-0">
        <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center">
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

        <div className="w-6 shrink-0 flex items-center justify-end">
          <button
            onClick={handleDelete}
            className="text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-colors cursor-pointer p-0"
            title="Delete session"
            aria-label={`Delete ${session.title || "session"}`}
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>

      {/* Row 2: Working Folder Path (left) + Timestamp (right, pixel-aligned with Delete button above) */}
      <div className="flex items-center justify-between gap-1 pl-5 min-w-0 text-[11px] text-foreground-muted mt-0.5">
        <div className="flex items-center gap-1 min-w-0 truncate">
          <FolderClosed size={11} className="shrink-0" />
          <span className="truncate">{basename(session.cwd)}</span>
        </div>
        <div className="w-6 shrink-0 flex items-center justify-end">
          <span className="text-[11px] text-foreground-muted">
            {formatRelativeTime(session.updatedAt, true)}
          </span>
        </div>
      </div>
    </div>
  );
});
