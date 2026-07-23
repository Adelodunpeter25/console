import React, { useState } from "react";
import { useSessions } from "@console/api";
import { activeProjectId$, activeSessionId$ } from "../../state/index.js";
import { observer } from "@legendapp/state/react";
import { Search, MessageSquare } from "lucide-react";

import { formatRelativeTime } from "../../utils/index.js";

export const ChatList = observer(() => {
  const [searchQuery, setSearchQuery] = useState("");
  const activeProjectId = activeProjectId$.get();
  const activeSessionId = activeSessionId$.get();

  const { data: sessions = [] } = useSessions(
    activeProjectId ? { projectId: activeProjectId } : undefined,
  );

  const filteredSessions = sessions.filter((s) =>
    (s.title || "New Chat").toLowerCase().includes(searchQuery.toLowerCase()),
  );

  if (!activeProjectId) return null;

  return (
    <div className="flex flex-col gap-2 p-2 border-t border-border/30">
      {/* Search Input Bar */}
      <div className="relative">
        <input
          type="text"
          placeholder="Search chats..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full bg-card/60 border border-border/50 rounded-md pl-7 pr-2 py-1.5 text-[11px] text-foreground focus:outline-none focus:border-primary/50"
        />
        <Search size={12} className="absolute left-2.5 top-2.5 text-muted-foreground" />
      </div>

      {/* Filtered Session Items */}
      <div className="flex flex-col gap-0.5 max-h-48 overflow-y-auto">
        {filteredSessions.map((sess) => {
          const isActive = sess.id === activeSessionId;
          const timeAgo = formatRelativeTime(sess.createdAt || sess.updatedAt);
          return (
            <div
              key={sess.id}
              onClick={() => activeSessionId$.set(sess.id)}
              className={`px-2.5 py-1.5 rounded-md text-xs flex items-center justify-between cursor-pointer transition-colors ${
                isActive
                  ? "bg-accent/80 text-foreground font-medium"
                  : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
              }`}
            >
              <div className="flex items-center gap-2 truncate">
                <MessageSquare size={12} className="text-primary/70 shrink-0" />
                <span className="truncate">{sess.title || "New Chat"}</span>
              </div>
              {timeAgo && (
                <span className="text-[10px] text-muted-foreground/70 font-mono shrink-0">
                  {timeAgo}
                </span>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
});
