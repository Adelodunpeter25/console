import React, { useEffect, useMemo } from "react";
import { useSession, runService } from "@console/api";
import { useConsoleStore } from "../../state/index";
import {
  useLocalRuntime,
  AssistantRuntimeProvider,
  type ChatModelAdapter,
  type ThreadMessageLike,
} from "@assistant-ui/react";
import { Thread } from "../assistant-ui/thread.js";
import { Sparkles } from "lucide-react";
import type { AgentMessage } from "@console/types";

function mapAgentMessageToAssistantMessage(msg: AgentMessage, idx: number): ThreadMessageLike {
  const id = (msg as { id?: string }).id || `msg-${idx}`;

  if (msg.role === "user") {
    return {
      id,
      role: "user",
      content: [{ type: "text", text: msg.content }],
    };
  }

  if (msg.role === "assistant") {
    const contentParts = msg.content.map((part) => {
      if (part.type === "text") {
        return { type: "text" as const, text: part.text };
      }
      if (part.type === "thinking") {
        return { type: "reasoning" as const, text: part.text };
      }
      if (part.type === "toolCall") {
        const args =
          part.call.arguments &&
          typeof part.call.arguments === "object" &&
          !Array.isArray(part.call.arguments)
            ? (part.call.arguments as Record<string, unknown>)
            : {};
        return {
          type: "tool-call" as const,
          toolCallId: part.call.id || `tool-${idx}`,
          toolName: part.call.name,
          args,
        };
      }
      return { type: "text" as const, text: "" };
    });

    return {
      id,
      role: "assistant",
      content: contentParts,
    };
  }

  if (msg.role === "toolResult") {
    // Render tool results as readable assistant text in the thread.
    const contentText = msg.results
      .map((res) => {
        const label = res.toolName || res.toolCallId || "tool";
        const body =
          typeof res.content === "string"
            ? res.content.slice(0, 1000)
            : JSON.stringify(res.content, null, 2).slice(0, 1000);
        return `*Tool Output (${label}):*\n\`\`\`json\n${body}\n\`\`\``;
      })
      .join("\n\n");

    return {
      id,
      role: "assistant",
      content: [{ type: "text", text: contentText }],
    };
  }

  return {
    id,
    role: "assistant",
    content: [{ type: "text", text: "" }],
  };
}

/**
 * Owns the assistant-ui runtime for a single session.
 * Remounted via `key={sessionId}` so history and adapter closures stay in sync.
 */
function SessionChatRuntime({ sessionId }: { sessionId: string }) {
  const { data: sessionData, isLoading, refetch: refetchSession } = useSession(sessionId);

  const initialMessages = useMemo(() => {
    if (!sessionData?.messages) return [];
    return sessionData.messages.map((m, idx) => mapAgentMessageToAssistantMessage(m, idx));
  }, [sessionData]);

  const chatAdapter: ChatModelAdapter = useMemo(
    () => ({
      async *run({ messages: runMessages, abortSignal }) {
        const latestMessage = runMessages[runMessages.length - 1];
        let prompt = "";
        if (latestMessage?.content && latestMessage.content.length > 0) {
          const textPart = latestMessage.content.find((p) => p.type === "text");
          if (textPart && "text" in textPart) {
            prompt = textPart.text;
          }
        }

        if (!prompt) {
          yield {
            content: [{ type: "text", text: "No prompt provided" }],
            status: { type: "incomplete", reason: "error", error: "No prompt provided" },
          };
          return;
        }

        if (abortSignal.aborted) {
          yield {
            content: [{ type: "text", text: "" }],
            status: { type: "incomplete", reason: "cancelled" },
          };
          return;
        }

        // Cancel in the UI aborts the fetch and tells the server to stop the run
        abortSignal.addEventListener(
          "abort",
          () => {
            void runService.abortRun(sessionId).catch(() => {});
          },
          { once: true },
        );

        const response = await runService.runSessionPrompt(sessionId, { prompt }, abortSignal);

        if (!response.ok) {
          let errorText = `Request failed (${response.status})`;
          try {
            const errBody = await response.json();
            if (errBody?.error) errorText = errBody.error;
          } catch {
            // ignore parse errors
          }
          yield {
            content: [{ type: "text", text: errorText }],
            status: { type: "incomplete", reason: "error", error: errorText },
          };
          return;
        }

        if (!response.body) {
          yield {
            content: [{ type: "text", text: "No response body from server" }],
            status: {
              type: "incomplete",
              reason: "error",
              error: "No response body from server",
            },
          };
          return;
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let accumulatedText = "";
        let buffer = "";
        let streamError: string | null = null;

        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split("\n");
            buffer = lines.pop() || "";

            for (const line of lines) {
              const trimmed = line.trim();
              if (!trimmed.startsWith("data: ")) continue;

              try {
                const eventFrame = JSON.parse(trimmed.slice(6));

                if (eventFrame.type === "error") {
                  streamError =
                    eventFrame.error?.message || eventFrame.message || "Agent run failed";
                  continue;
                }

                if (eventFrame.type === "modelStreamPart" && eventFrame.part?.text) {
                  accumulatedText += eventFrame.part.text;
                  yield {
                    content: [{ type: "text", text: accumulatedText }],
                    status: { type: "running" },
                  };
                }
              } catch {
                // Ignore partial/non-JSON frames
              }
            }
          }
        } finally {
          reader.releaseLock();
          // Refresh persisted history (tool calls, final turns) after stream ends
          void refetchSession();
        }

        if (streamError) {
          yield {
            content: [
              {
                type: "text",
                text: accumulatedText ? `${accumulatedText}\n\n⚠️ ${streamError}` : streamError,
              },
            ],
            status: { type: "incomplete", reason: "error", error: streamError },
          };
          return;
        }

        yield {
          content: [{ type: "text", text: accumulatedText }],
          status: { type: "complete", reason: "stop" },
        };
      },
    }),
    [sessionId, refetchSession],
  );

  const runtime = useLocalRuntime(chatAdapter, {
    initialMessages,
  });

  // When history arrives/changes after mount (or after refetch), sync the thread.
  // Skip while a run is in progress so we don't clobber the live stream.
  useEffect(() => {
    if (!sessionData?.messages) return;
    if (runtime.thread.getState().isRunning) return;

    const mapped = sessionData.messages.map((m, idx) => mapAgentMessageToAssistantMessage(m, idx));
    runtime.thread.reset(mapped);
  }, [sessionData, runtime]);

  if (isLoading && !sessionData) {
    return (
      <div className="flex-1 flex items-center justify-center text-muted-foreground text-sm">
        Loading session…
      </div>
    );
  }

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <div className="flex-1 flex flex-col h-screen bg-background relative overflow-hidden text-foreground">
        <Thread />
      </div>
    </AssistantRuntimeProvider>
  );
}

export const AssistantChat = () => {
  const activeSessionId = useConsoleStore((s) => s.activeSessionId);

  if (!activeSessionId) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-center text-muted-foreground bg-background">
        <Sparkles className="w-12 h-12 opacity-30 mb-4 text-primary animate-pulse" />
        <h2 className="text-xl font-medium text-foreground mb-1">No Active Chat Session</h2>
        <p className="text-sm max-w-sm">
          Select an existing chat or create a new session from the sidebar to begin pair
          programming.
        </p>
      </div>
    );
  }

  // key forces a fresh runtime when switching sessions
  return <SessionChatRuntime key={activeSessionId} sessionId={activeSessionId} />;
});
