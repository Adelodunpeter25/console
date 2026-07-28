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
  const status = session.status ?? "idle";

  return (
    <div
      className={`group mx-2 flex flex-col gap-0.5 px-3 py-1.5 rounded-lg cursor-pointer transition-colors ${
        isActive ? "bg-white/10" : "hover:bg-white/[0.04]"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Row 1: status indicator + title + timestamp */}
      <div className="flex items-center gap-2">
        {status === "working" ? (
          <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center">
            <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_DOT[status]}`} />
        )}

        <span
          className={`text-xs font-medium truncate flex-1 ${
            isActive ? "text-foreground" : "text-foreground-secondary"
          }`}
        >
          {session.title || "Untitled"}
        </span>

        <span className="text-xs text-foreground-muted shrink-0">
          {formatRelativeTime(session.createdAt)}
        </span>

        <button
          onClick={(e) => {
            e.stopPropagation();
            deleteSession(session.id, projectId);
            if (isActive) setSelectedSessionId(null);
          }}
          className="text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity shrink-0"
        >
          <X size={12} />
        </button>
      </div>

      {/* Row 2: branch name */}
      <span className="text-xs text-foreground-muted font-mono pl-6">
        main
      </span>
    </div>
  );
}
