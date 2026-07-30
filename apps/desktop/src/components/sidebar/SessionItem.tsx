import type { SessionHeader, SessionStatus } from "@console/types";
import { Trash2 } from "lucide-react";
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
 * Single session row in the sidebar — status dot/spinner, title, timestamp, and hover Delete button.
 * Row height increased to py-2.5 for comfortable interaction.
 */
export function SessionItem({ session, projectId, isActive }: SessionItemProps) {
  const { setSelectedSessionId } = useAppStore();
  const { deleteSession } = useProjectStore();
  const status: SessionStatus = session.status ?? "idle";

  return (
    <div
      className={`group relative flex items-center justify-between px-3 py-2.5 rounded-md cursor-pointer ${
        isActive
          ? "bg-white/[0.08] text-foreground font-medium"
          : "text-foreground-secondary hover:text-foreground"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Status Dot / Spinner */}
      {status === "working" ? (
        <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center mr-2">
          <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : (
        <div className={`w-1.5 h-1.5 rounded-full shrink-0 mr-2.5 ${STATUS_DOT[status] ?? STATUS_DOT.idle}`} />
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
