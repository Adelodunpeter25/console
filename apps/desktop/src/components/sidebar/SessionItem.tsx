import type { SessionHeader, SessionStatus } from "@console/types";
import { X } from "lucide-react";
import { useAppStore, useProjectStore } from "../../store";
import { formatRelativeTime } from "../../utils/time";

const STATUS_DOT: Record<SessionStatus, string> = {
  idle: "bg-foreground-muted",
  working: "bg-blue-500",
  done: "bg-green-500",
  needs_attention: "bg-amber-500",
};

interface SessionItemProps {
  session: SessionHeader;
  projectId: string;
  isActive: boolean;
}

/**
 * Single session row in the sidebar — status dot, title, timestamp, delete.
 */
export function SessionItem({ session, projectId, isActive }: SessionItemProps) {
  const { setSelectedSessionId } = useAppStore();
  const { deleteSession } = useProjectStore();

  return (
    <div
      className={`group relative flex items-center justify-between px-3 py-1.5 rounded-md cursor-pointer transition-colors ${
        isActive
          ? "bg-white/[0.08] text-foreground font-medium"
          : "text-foreground-secondary hover:bg-white/[0.04] hover:text-foreground"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Active Indicator Bar */}
      {isActive && (
        <div className="absolute left-0 top-1.5 bottom-1.5 w-1 bg-white rounded-r" />
      )}

      {/* Title */}
      <span className="text-xs truncate flex-1 pr-2">
        {session.title || "Untitled Chat"}
      </span>

      {/* Relative Time */}
      <span className="text-[11px] text-foreground-muted shrink-0 group-hover:hidden">
        {formatRelativeTime(session.createdAt, true)}
      </span>

      {/* Hover Delete Action */}
      <button
        onClick={(e) => {
          e.stopPropagation();
          deleteSession(session.id, projectId);
          if (isActive) setSelectedSessionId(null);
        }}
        className="hidden group-hover:flex items-center text-foreground-muted hover:text-danger transition-colors shrink-0"
        title="Delete session"
      >
        <X size={13} />
      </button>
    </div>
  );
}
