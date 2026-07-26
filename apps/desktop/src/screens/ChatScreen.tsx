import React from "react";
import { ArrowUp, Square, Paperclip, Brain, AlertCircle } from "lucide-react";
import type { AgentMessage, AssistantMessageContent, ToolCall } from "@console/types";
import { useAppStore, useChatStore, useProjectStore, useProviderStore } from "../store";
import { MarkdownRenderer } from "../components/MarkdownRenderer";
import { ModelSelector } from "../components/ModelSelector";
import { ToolCallBlock } from "../components/ToolCallBlock";

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
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const [selectedModel, setSelectedModel] = React.useState<string | null>(null);
  const [autoScroll, setAutoScroll] = React.useState(true);

  // Load providers for the model selector
  const { loadProviders } = useProviderStore();
  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    if (selectedSessionId) {
      loadSession(selectedSessionId);
    } else {
      clear();
    }
  }, [selectedSessionId, loadSession, clear]);

  // Auto-scroll to bottom when new content arrives (if user hasn't scrolled up)
  React.useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streamingText, streamingThinking, autoScroll]);

  // Auto-grow textarea
  React.useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 160)}px`;
    }
  }, [input]);

  const handleScroll = () => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const atBottom = scrollHeight - scrollTop - clientHeight < 60;
    setAutoScroll(atBottom);
  };

  const handleSend = () => {
    if (!selectedSessionId || !input.trim() || running) return;
    setAutoScroll(true);
    sendMessage(selectedSessionId);
  };

  const projectName = projects.find((p) => p.id === selectedProjectId)?.name;

  if (!selectedSessionId) {
    return (
      <div className="flex-1 flex items-center justify-center bg-screen">
        <div className="text-center max-w-sm">
          <div className="w-14 h-14 rounded-2xl bg-card border border-border flex items-center justify-center mx-auto mb-4">
            <Brain size={26} className="text-foreground-muted" />
          </div>
          <p className="text-foreground text-base font-semibold mb-1.5">No Session Selected</p>
          <p className="text-foreground-secondary text-sm">
            Select or create a chat session from the sidebar to start chatting with the agent.
          </p>
        </div>
      </div>
    );
  }

  // Collect pending tool calls from the latest assistant message for display
  const lastAssistantMsg = [...messages].reverse().find((m) => m.role === "assistant");
  const pendingToolCalls: ToolCall[] =
    lastAssistantMsg?.role === "assistant"
      ? lastAssistantMsg.content
          .filter((c): c is Extract<AssistantMessageContent, { type: "toolCall" }> => c.type === "toolCall")
          .map((c) => c.call)
      : [];

  return (
    <div className="flex-1 flex flex-col h-full bg-screen">
      {/* Messages area */}
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto"
      >
        {messages.length === 0 && !streamingText && !streamingThinking ? (
          <div className="flex items-center justify-center h-full">
            <p className="text-foreground-muted text-sm">
              Type a prompt below to start the agent.
            </p>
          </div>
        ) : (
          <div className="max-w-3xl mx-auto px-6 py-6 space-y-4">
            {messages.map((msg, i) => (
              <MessageBubble
                key={msg.role === "assistant" && msg.id ? msg.id : `${msg.role}-${i}`}
                message={msg}
                prevMessage={messages[i - 1]}
              />
            ))}
            {(streamingText || streamingThinking) && (
              <StreamingBubble text={streamingText} thinking={streamingThinking} />
            )}
            {running && pendingToolCalls.length > 0 && !streamingText && (
              <ToolCallBlock calls={pendingToolCalls} />
            )}
          </div>
        )}
      </div>

      {/* Scroll-to-bottom button */}
      {!autoScroll && (streamingText || streamingThinking || running) && (
        <div className="relative">
          <button
            onClick={() => {
              setAutoScroll(true);
              if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
            }}
            className="absolute -top-12 left-1/2 -translate-x-1/2 px-3 py-1.5 bg-card border border-border-strong rounded-full text-xs font-medium text-foreground-secondary shadow-lg hover:text-foreground transition-colors z-10"
          >
            ↓ Latest
          </button>
        </div>
      )}

      {/* Composer */}
      <div className="px-6 pb-4 pt-2">
        <div className="max-w-3xl mx-auto">
          <div className="bg-card border border-border rounded-2xl focus-within:border-border-strong transition-colors">
            {/* Text input */}
            <textarea
              ref={textareaRef}
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
              className="w-full bg-transparent px-4 pt-3.5 pb-2 text-sm text-foreground placeholder:text-foreground-muted outline-none resize-none"
              style={{ minHeight: "44px", maxHeight: "160px" }}
            />

            {/* Toolbar */}
            <div className="flex items-center gap-1 px-3 pb-2.5">
              <ModelSelector value={selectedModel} onChange={setSelectedModel} />

              <button
                className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-foreground-muted hover:text-foreground-secondary hover:bg-white/5 transition-colors"
                title="Attach file (coming soon)"
              >
                <Paperclip size={14} />
              </button>

              <div className="flex-1" />

              {projectName && (
                <span className="text-xs text-foreground-muted font-mono mr-1 hidden sm:inline">
                  {projectName}
                </span>
              )}

              {running ? (
                <button
                  onClick={() => selectedSessionId && abort(selectedSessionId)}
                  className="w-8 h-8 rounded-full bg-danger/80 flex items-center justify-center text-white hover:bg-danger transition-colors"
                  title="Stop (Esc)"
                >
                  <Square size={14} fill="currentColor" />
                </button>
              ) : (
                <button
                  onClick={handleSend}
                  disabled={!input.trim()}
                  className="w-8 h-8 rounded-full bg-foreground flex items-center justify-center text-black disabled:opacity-20 hover:bg-foreground/90 transition-all"
                  title="Send (Enter)"
                >
                  <ArrowUp size={16} strokeWidth={2.5} />
                </button>
              )}
            </div>
          </div>
          <p className="text-xs text-foreground-muted text-center mt-2">
            <kbd className="font-mono">Enter</kbd> to send ·{" "}
            <kbd className="font-mono">Shift+Enter</kbd> for newline
          </p>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Message bubbles
// ---------------------------------------------------------------------------

function MessageBubble({
  message,
  prevMessage,
}: {
  message: AgentMessage;
  prevMessage?: AgentMessage;
}) {
  if (message.role === "user") {
    return <UserBubble content={message.content} />;
  }

  if (message.role === "toolResult") {
    // Tool results are rendered inline after the assistant message that
    // contained the tool calls — find the matching calls from the previous
    // assistant message.
    const prevCalls: ToolCall[] =
      prevMessage?.role === "assistant"
        ? prevMessage.content
            .filter(
              (c): c is Extract<AssistantMessageContent, { type: "toolCall" }> =>
                c.type === "toolCall",
            )
            .map((c) => c.call)
        : [];
    return <ToolCallBlock calls={prevCalls} results={message.results} />;
  }

  // assistant
  return <AssistantBubble message={message} />;
}

function UserBubble({ content }: { content: string }) {
  return (
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-2xl bg-user-bubble border border-user-bubble-border px-4 py-3">
        <p className="text-sm text-foreground whitespace-pre-wrap break-words">{content}</p>
      </div>
    </div>
  );
}

function AssistantBubble({
  message,
}: {
  message: Extract<AgentMessage, { role: "assistant" }>;
}) {
  // Check if this message is an error (text starts with "Error:")
  const textParts = message.content.filter((c) => c.type === "text");
  const thinkingParts = message.content.filter((c) => c.type === "thinking");
  const toolCallParts = message.content.filter((c) => c.type === "toolCall");
  const isError = textParts.some((c) => c.type === "text" && c.text.startsWith("Error:"));

  if (isError) {
    return (
      <div className="rounded-xl bg-danger-muted border border-danger/30 px-4 py-3">
        <div className="flex items-start gap-2.5">
          <AlertCircle size={16} className="text-danger shrink-0 mt-0.5" />
          <div className="text-sm text-danger">
            {textParts.map((c, i) => (
              <p key={i} className="font-mono">
                {c.type === "text" && c.text}
              </p>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {/* Thinking parts */}
      {thinkingParts.map((part, i) => (
        <div
          key={`thinking-${i}`}
          className="rounded-lg bg-thinking border border-thinking-border px-3.5 py-2.5"
        >
          <div className="flex items-center gap-1.5 mb-1">
            <Brain size={12} className="text-purple-400" />
            <span className="text-xs font-semibold text-purple-400 uppercase tracking-wider">
              Thinking
            </span>
          </div>
          <p className="text-sm text-foreground-secondary italic whitespace-pre-wrap break-words">
            {part.type === "thinking" && part.text}
          </p>
        </div>
      ))}

      {/* Text parts rendered as markdown */}
      {textParts.length > 0 && (
        <div className="px-1">
          <MarkdownRenderer
            content={textParts
              .map((c) => (c.type === "text" ? c.text : ""))
              .join("\n\n")}
          />
        </div>
      )}

      {/* Tool calls (inline preview, results come as separate message) */}
      {toolCallParts.length > 0 && (
        <ToolCallBlock
          calls={toolCallParts
            .map((c) => (c.type === "toolCall" ? c.call : null))
            .filter((c): c is ToolCall => c !== null)}
        />
      )}
    </div>
  );
}

function StreamingBubble({ text, thinking }: { text: string; thinking: string }) {
  return (
    <div className="space-y-2">
      {thinking && (
        <div className="rounded-lg bg-thinking border border-thinking-border px-3.5 py-2.5">
          <div className="flex items-center gap-1.5 mb-1">
            <Brain size={12} className="text-purple-400" />
            <span className="text-xs font-semibold text-purple-400 uppercase tracking-wider">
              Thinking
            </span>
          </div>
          <p className="text-sm text-foreground-secondary italic whitespace-pre-wrap break-words">
            {thinking}
          </p>
        </div>
      )}
      {text && (
        <div className="px-1">
          <MarkdownRenderer content={text} />
        </div>
      )}
      {!text && !thinking && (
        <div className="flex items-center gap-2 text-foreground-muted">
          <div className="w-2 h-2 rounded-full bg-foreground-muted animate-pulse" />
          <span className="text-xs">Agent is thinking...</span>
        </div>
      )}
    </div>
  );
}
