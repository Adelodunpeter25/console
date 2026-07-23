import React, { useEffect, useState } from "react";
import { useSession } from "@console/api";
import { activeSessionId$ } from "../../state/index.js";
import { observer } from "@legendapp/state/react";
import {
  useLocalRuntime,
  AssistantRuntimeProvider,
  type ChatModelAdapter,
} from "@assistant-ui/react";
import { Thread } from "@assistant-ui/react";
import { Sparkles } from "lucide-react";
import type { AgentMessage } from "@console/types";

function mapAgentMessageToAssistantMessage(msg: AgentMessage, idx: number) {
  const id = (msg as any).id || `msg-${idx}`;

  if (msg.role === "user") {
    return {
      id,
      role: "user" as const,
      content: [{ type: "text" as const, text: msg.content }],
    };
  }

  if (msg.role === "assistant") {
    const contentParts = msg.content.map((part) => {
      if (part.type === "text") {
        return { type: "text" as const, text: part.text };
      }
      if (part.type === "thinking") {
        return { type: "text" as const, text: `*Thinking: ${part.text}*` };
      }
      if (part.type === "toolCall") {
        return {
          type: "text" as const,
          text: `*Running Tool: ${part.call.name}(${JSON.stringify(part.call.arguments)})*`,
        };
      }
      return { type: "text" as const, text: "" };
    });

    return {
      id,
      role: "assistant" as const,
      content: contentParts,
    };
  }

  if (msg.role === "toolResult") {
    const contentText = msg.results
      .map((res) => {
        return `*Tool Output (${res.name}):* \n\`\`\`json\n${typeof res.content === "string" ? res.content.slice(0, 1000) : JSON.stringify(res.content, null, 2).slice(0, 1000)}\n\`\`\``;
      })
      .join("\n\n");

    return {
      id,
      role: "assistant" as const,
      content: [{ type: "text" as const, text: contentText }],
    };
  }

  return {
    id,
    role: "assistant" as const,
    content: [{ type: "text" as const, text: "" }],
  };
}

export const AssistantChat = observer(() => {
  const activeSessionId = activeSessionId$.get();

  const { data: sessionData, refetch: refetchSession } = useSession(activeSessionId || "");
  const [messages, setMessages] = useState<any[]>([]);

  // Load existing session messages when session data loads/changes
  useEffect(() => {
    if (sessionData && sessionData.messages) {
      const mapped = sessionData.messages.map((m, idx) =>
        mapAgentMessageToAssistantMessage(m, idx),
      );
      setMessages(mapped);
    } else {
      setMessages([]);
    }
  }, [sessionData]);

  // Adapter for the custom SSE endpoint
  const chatAdapter: ChatModelAdapter = {
    async *run({ messages: runMessages, abortSignal }) {
      const latestMessage = runMessages[runMessages.length - 1];
      let prompt = "";
      if (latestMessage.content && latestMessage.content.length > 0) {
        const textPart = latestMessage.content.find((p) => p.type === "text");
        if (textPart && "text" in textPart) {
          prompt = textPart.text;
        }
      }

      if (!prompt || !activeSessionId) {
        yield {
          content: [{ type: "text", text: "No prompt provided" }],
          status: { type: "complete", reason: "stop" },
        };
        return;
      }

      // Add user message to local state immediately
      setMessages((prev) => [
        ...prev,
        { id: `user-msg-${Date.now()}`, role: "user", content: [{ type: "text", text: prompt }] },
      ]);

      const response = await fetch(`http://localhost:3000/api/sessions/${activeSessionId}/run`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt }),
        signal: abortSignal,
      });

      if (!response.body) {
        throw new Error("No response body");
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let accumulatedText = "";
      let buffer = "";

      // Add temporary response placeholder
      const tempResponseId = `assistant-msg-${Date.now()}`;
      setMessages((prev) => [
        ...prev,
        { id: tempResponseId, role: "assistant", content: [{ type: "text", text: "" }] },
      ]);

      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";

          for (const line of lines) {
            const trimmed = line.trim();
            if (trimmed.startsWith("data: ")) {
              try {
                const eventFrame = JSON.parse(trimmed.slice(6));

                // Real-time UI updates
                if (eventFrame.type === "modelStreamPart" && eventFrame.part?.text) {
                  accumulatedText += eventFrame.part.text;

                  setMessages((prev) =>
                    prev.map((m) => {
                      if (m.id === tempResponseId) {
                        return { ...m, content: [{ type: "text", text: accumulatedText }] };
                      }
                      return m;
                    }),
                  );

                  yield {
                    content: [{ type: "text", text: accumulatedText }],
                    status: { type: "running" },
                  };
                }
              } catch (e) {
                // Ignore parse errors
              }
            }
          }
        }
      } finally {
        reader.releaseLock();
        refetchSession(); // Refresh complete logs from database
      }

      yield {
        content: [{ type: "text", text: accumulatedText }],
        status: { type: "complete", reason: "stop" },
      };
    },
  };

  const runtime = useLocalRuntime(chatAdapter, {
    initialMessages: messages,
  });

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

  return (
    <AssistantRuntimeProvider runtime={runtime}>
      <div className="flex-1 flex flex-col h-screen bg-background relative overflow-hidden text-foreground">
        <Thread />
      </div>
    </AssistantRuntimeProvider>
  );
});
