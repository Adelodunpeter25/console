import React from "react";
import { ArrowUp, Square } from "lucide-react";
import type { AgentMessage, AssistantMessageContent } from "@console/types";
import { useAppStore, useChatStore, useProjectStore } from "../store";

export function ChatScreen() {
  const { selectedSessionId, selectedProjectId } = useAppStore();
  const {
    messages,
    input,
    running,
    streamingText,
    streamingThinking,
    setInput,
    sendMessage,
    abort,
    loadSession,
    clear,
  } = useChatStore();
  const { projects } = useProjectStore();

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
  }, [messages, streamingText, streamingThinking]);

  const handleSend = () => {
    if (!selectedSessionId || !input.trim() || running) return;
    sendMessage(selectedSessionId);
  };

  const projectName = projects.find((p) => p.id === selectedProjectId)?.name;

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
        {messages.length === 0 && !streamingText && !streamingThinking ? (
          <div className="flex items-center justify-center py-14">
            <p className="text-foreground-muted text-sm italic">
              No messages yet. Type a prompt below to start.
            </p>
          </div>
        ) : (
          <div className="space-y-3.5 max-w-4xl mx-auto">
            {messages.map((msg, i) => (
              <MessageBubble
                key={msg.role === "assistant" && msg.id ? msg.id : `${msg.role}-${i}`}
                message={msg}
              />
            ))}
            {(streamingText || streamingThinking) && (
              <StreamingBubble text={streamingText} thinking={streamingThinking} />
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
        {projectName && (
          <p className="text-xs text-foreground-muted mt-2 text-center">
            Project: {projectName}
          </p>
        )}
      </div>
    </div>
  );
}

function StreamingBubble({ text, thinking }: { text: string; thinking: string }) {
  return (
    <div className="p-4 rounded-xl bg-card border border-border w-full">
      <div className="text-xs font-bold text-foreground-muted uppercase mb-1.5">Agent</div>
      {thinking && (
        <div className="text-foreground-muted text-sm italic whitespace-pre-wrap break-words mb-2 border-l-2 border-border pl-3">
          {thinking}
        </div>
      )}
      <div className="text-foreground text-sm leading-6 whitespace-pre-wrap break-words">
        {text || "..."}
      </div>
    </div>
  );
}

function MessageBubble({ message }: { message: AgentMessage }) {
  const isUser = message.role === "user";

  if (isUser) {
    return (
      <div className="p-4 rounded-xl bg-white/10 border border-border self-end max-w-[85%] ml-auto">
        <div className="text-xs font-bold text-foreground-muted uppercase mb-1.5">You</div>
        <div className="text-foreground text-sm leading-6 whitespace-pre-wrap break-words">
          {message.content}
        </div>
      </div>
    );
  }

  if (message.role === "toolResult") {
    return (
      <div className="p-4 rounded-xl bg-card-alt border border-border w-full">
        <div className="text-xs font-bold text-foreground-muted uppercase mb-1.5">Tool</div>
        <div className="space-y-2">
          {message.results.map((r, i) => (
            <ToolResultRow
              key={r.toolCallId ?? i}
              name={r.toolName ?? "tool"}
              content={r.content}
              isError={r.isError}
            />
          ))}
        </div>
      </div>
    );
  }

  // assistant
  return (
    <div className="p-4 rounded-xl bg-card border border-border w-full">
      <div className="text-xs font-bold text-foreground-muted uppercase mb-1.5">Agent</div>
      <div className="space-y-2">
        {message.content.map((part, i) => (
          <AssistantPart key={i} part={part} />
        ))}
      </div>
    </div>
  );
}

function AssistantPart({ part }: { part: AssistantMessageContent }) {
  if (part.type === "text") {
    return (
      <div className="text-foreground text-sm leading-6 whitespace-pre-wrap break-words">
        {part.text}
      </div>
    );
  }
  if (part.type === "thinking") {
    return (
      <div className="text-foreground-muted text-sm italic whitespace-pre-wrap break-words border-l-2 border-border pl-3">
        {part.text}
      </div>
    );
  }
  // toolCall
  const args = part.call.arguments;
  const argsText =
    typeof args === "string" ? args : JSON.stringify(args, null, 2);
  return (
    <div className="text-foreground-secondary text-xs font-mono whitespace-pre-wrap break-words border border-border rounded-lg px-3 py-2 bg-screen">
      <span className="text-foreground font-bold">→ {part.call.name}</span>
      <span className="text-foreground-muted">(</span>
      <span className="text-foreground-secondary">{argsText}</span>
      <span className="text-foreground-muted">)</span>
    </div>
  );
}

function ToolResultRow({
  name,
  content,
  isError,
}: {
  name: string;
  content: unknown;
  isError?: boolean;
}) {
  const text =
    typeof content === "string" ? content : JSON.stringify(content, null, 2);
  return (
    <div
      className={`text-xs font-mono whitespace-pre-wrap break-words rounded-lg px-3 py-2 border ${
        isError
          ? "border-danger/40 bg-danger/10 text-danger"
          : "border-border bg-screen text-foreground-secondary"
      }`}
    >
      <span className={`font-bold ${isError ? "text-danger" : "text-foreground"}`}>{name}</span>
      <span className="text-foreground-muted"> →</span>
      <pre className="mt-1 whitespace-pre-wrap break-words">{text}</pre>
    </div>
  );
}
