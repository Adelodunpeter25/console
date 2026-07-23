import React, { useState } from "react";
import { useSessions } from "@console/api";
import { globalState$ } from "../../state/global-state.js";
import { observer } from "@legendapp/state/react";
import { Search, MessageSquare } from "lucide-react";

export const ChatList = observer(() => {
  const [searchQuery, setSearchQuery] = useState("");
  const activeProjectId = globalState$.activeProjectId.get();
  const activeSessionId = globalState$.activeSessionId.get();

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
          className="w-full bg-card/60 border border-border/50 rounded-md pl-7 pr-2 py-1 text-[11px] text-foreground focus:outline-none focus:border-primary/50"
        />
        <Search size={12} className="absolute left-2.5 top-2 text-muted-foreground" />
      </div>

      {/* Filtered Session Items */}
      <div className="flex flex-col gap-0.5 max-h-40 overflow-y-auto">
        {filteredSessions.map((sess) => {
          const isActive = sess.id === activeSessionId;
          return (
            <div
              key={sess.id}
              onClick={() => globalState$.activeSessionId.set(sess.id)}
              className={`px-2 py-1 rounded text-xs flex items-center gap-2 cursor-pointer transition-colors ${
                isActive
                  ? "bg-accent/80 text-foreground font-medium"
                  : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
              }`}
            >
              <MessageSquare size={12} className="text-primary/70 shrink-0" />
              <span className="truncate">{sess.title || "New Chat"}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
});
