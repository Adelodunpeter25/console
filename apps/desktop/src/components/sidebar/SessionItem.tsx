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

      {/* Right Slot: Timestamp & Hover Delete Icon */}
      <div className="relative shrink-0 flex items-center justify-end">
        <span className="text-[11px] text-foreground-muted group-hover:opacity-0 transition-opacity">
          {formatRelativeTime(session.createdAt, true)}
        </span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            deleteSession(session.id, projectId);
            if (isActive) setSelectedSessionId(null);
          }}
          className="absolute inset-0 flex items-center justify-end text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
          title="Delete session"
        >
          <Trash2 size={13} />
        </button>
      </div>
    </div>
  );
}
