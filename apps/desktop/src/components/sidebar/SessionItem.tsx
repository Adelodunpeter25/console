import type { SessionHeader, SessionStatus } from "@console/types";
import { Folder, Trash2 } from "lucide-react";
import { useAppStore, useProjectStore, useSessionStatusStore } from "../../store";
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
export function SessionItem({ session, isActive }: SessionItemProps) {
  const { setSelectedSessionId } = useAppStore();
  const { deleteSession } = useProjectStore();
  const liveStatus = useSessionStatusStore((state) => state.statuses[session.id]);
  const status: SessionStatus = liveStatus ?? session.status ?? "idle";

  return (
    <div
      className={`group relative flex flex-col px-3 py-2 rounded-md cursor-pointer ${
        isActive
          ? "bg-white/[0.08] text-foreground"
          : "text-foreground-secondary hover:text-foreground"
      }`}
      onClick={() => setSelectedSessionId(isActive ? null : session.id)}
    >
      {/* Line 1: Status Dot + Title + Time */}
      <div className="flex items-center gap-2 min-w-0">
        {status === "working" ? (
          <div className="w-3.5 h-3.5 shrink-0 flex items-center justify-center">
            <div className="w-3 h-3 border-[1.5px] border-blue-500 border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${STATUS_DOT[status] ?? STATUS_DOT.idle}`} />
        )}

        <span className={`text-xs truncate flex-1 min-w-0 ${isActive ? "font-medium" : ""}`}>
          {session.title || "Untitled Chat"}
        </span>

        <span className="text-[11px] text-foreground-muted shrink-0">
          {formatRelativeTime(session.updatedAt, true)}
        </span>

        <button
          onClick={(e) => {
            e.stopPropagation();
            deleteSession(session.id);
            if (isActive) setSelectedSessionId(null);
          }}
          className="w-4 h-4 shrink-0 flex items-center justify-center text-foreground-muted hover:text-danger opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
          title="Delete session"
          aria-label={`Delete ${session.title || "session"}`}
        >
          <Trash2 size={13} />
        </button>
      </div>

      {/* Line 2: Working Folder */}
      <div className="flex items-center gap-1.5 mt-0.5 pl-[18px] pr-5 min-w-0">
        <Folder size={11} className="text-foreground-muted shrink-0" />
        <span className="text-[11px] text-foreground-muted truncate min-w-0">
          {basename(session.cwd)}
        </span>
      </div>
    </div>
  );
}
