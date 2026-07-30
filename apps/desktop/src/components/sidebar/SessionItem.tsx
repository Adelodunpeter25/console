import type { SessionHeader } from "@console/types";
import { Trash2 } from "lucide-react";
import { useAppStore, useProjectStore } from "../../store";
import { formatRelativeTime } from "../../utils/time";

interface SessionItemProps {
  session: SessionHeader;
  projectId: string;
  isActive: boolean;
}

/**
 * Single session row in the sidebar — title, timestamp, and hover Delete button.
 */
export function SessionItem({ session, projectId, isActive }: SessionItemProps) {
  const { setSelectedSessionId } = useAppStore();
  const { deleteSession } = useProjectStore();

  return (
    <div
      className={`group relative flex items-center justify-between px-3 py-1.5 rounded-md cursor-pointer ${
        isActive
          ? "bg-white/[0.08] text-foreground font-medium"
          : "text-foreground-secondary hover:text-foreground"
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

      {/* Hover Delete Action Button */}
      <button
        onClick={(e) => {
          e.stopPropagation();
          deleteSession(session.id, projectId);
          if (isActive) setSelectedSessionId(null);
        }}
        className="hidden group-hover:flex items-center text-foreground-muted hover:text-danger transition-colors shrink-0"
        title="Delete session"
      >
        <Trash2 size={13} />
      </button>
    </div>
  );
}
