import React from "react";
import { ArrowUp, Square } from "lucide-react";
import type { AgentMessage } from "@console/types";
import { useAppStore, useChatStore } from "../store";

export function ChatScreen() {
  const { selectedSessionId, selectedProjectId } = useAppStore();
  const {
    messages,
    input,
    running,
    streamingText,
    setInput,
    sendMessage,
    abort,
    loadSession,
    clear,
  } = useChatStore();

  const scrollRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadSession(selectedSessionId);
    } else {
      clear();
    }
  }, [selectedSessionId, loadSession, clear]);

  React.useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streamingText]);

  const handleSend = () => {
    if (!selectedSessionId || !input.trim() || running) return;
    sendMessage(selectedSessionId);
  };

  if (!selectedSessionId) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="text-center max-w-sm">
          <p className="text-foreground text-lg font-semibold mb-2">No Session Selected</p>
          <p className="text-foreground-secondary text-sm">
            Select or create a chat session from the sidebar to start chatting with the agent.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col h-full">
      {/* Messages area */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto px-6 py-4">
        {messages.length === 0 && !streamingText ? (
          <div className="flex items-center justify-center py-14">
            <p className="text-foreground-muted text-sm italic">
              No messages yet. Type a prompt below to start.
            </p>
          </div>
        ) : (
          <div className="space-y-3.5 max-w-4xl mx-auto">
            {messages.map((msg, i) => (
              <MessageBubble key={i} message={msg} />
            ))}
            {streamingText && (
              <MessageBubble
                message={{
                  role: "assistant",
                  content: [{ type: "text", text: streamingText }],
                }}
                streaming
              />
            )}
          </div>
        )}
      </div>

      {/* Input bar */}
      <div className="px-6 py-3.5 bg-screen-alt border-t border-border">
        <div className="flex gap-2.5 items-end max-w-4xl mx-auto">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Ask agent to write code..."
            rows={1}
            className="flex-1 min-h-11 max-h-32 bg-card-alt border border-border rounded-xl px-4 py-2.5 text-sm text-foreground outline-none focus:border-white/30 transition-colors resize-none"
          />
          {running ? (
            <button
              onClick={() => selectedSessionId && abort(selectedSessionId)}
              className="h-11 bg-danger/80 rounded-xl px-5 flex items-center justify-center text-white text-sm font-bold hover:bg-danger transition-colors"
            >
              <Square size={16} />
            </button>
          ) : (
            <button
              onClick={handleSend}
              disabled={!input.trim() || !selectedSessionId}
              className="h-11 bg-white rounded-xl px-5 flex items-center justify-center text-black text-sm font-bold disabled:opacity-30 hover:bg-white/90 transition-colors"
            >
              <ArrowUp size={18} />
            </button>
          )}
        </div>
        {selectedProjectId && (
          <p className="text-xs text-foreground-muted mt-2 text-center">
            Project: {selectedProjectId}
          </p>
        )}
      </div>
    </div>
  );
}

function MessageBubble({ message, streaming }: { message: AgentMessage; streaming?: boolean }) {
  const isUser = message.role === "user";

  let text = "";
  if (message.role === "user") {
    text = message.content;
  } else if (message.role === "assistant") {
    text = message.content
      .map((c) => (c.type === "text" || c.type === "thinking" ? c.text : ""))
      .join("\n");
  } else if (message.role === "toolResult") {
    text = message.results
      .map((r) => `${r.toolName ?? "tool"}: ${typeof r.content === "string" ? r.content : JSON.stringify(r.content)}`)
      .join("\n");
  }

  return (
    <div
      className={`p-4 rounded-xl ${
        isUser
          ? "bg-white/10 border border-border self-end max-w-[85%] ml-auto"
          : streaming
            ? "bg-card border border-border w-full"
            : "bg-card border border-border w-full"
      }`}
    >
      <div className="text-xs font-bold text-foreground-muted uppercase mb-1.5">
        {isUser ? "You" : message.role === "toolResult" ? "Tool" : "Agent"}
      </div>
      <div className="text-foreground text-sm leading-6 whitespace-pre-wrap break-words">
        {text || (streaming ? "..." : "")}
      </div>
    </div>
  );
}
