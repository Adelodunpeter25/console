import React from "react";
import { View, Text, ActivityIndicator } from "react-native";
import type { AgentMessage, ToolResult } from "@console/types";
import { MarkdownRenderer } from "../common/markdown-renderer";
import { theme } from "../../styles/theme";

export function ToolResultItem({ result }: { result: ToolResult }) {
  const isError = result.isError;
  return (
    <View className="bg-card-alt border border-border rounded-lg p-2.5 mb-1.5">
      <View className="flex-row justify-between items-center mb-1">
        <Text className="text-xs font-mono font-bold text-foreground">⚙️ {result.toolName}</Text>
        <Text
          className="text-[9px] font-bold font-mono tracking-wide"
          style={{ color: isError ? theme.colors.status.attention : theme.colors.status.ready }}
        >
          {isError ? "FAILED" : "DONE"}
        </Text>
      </View>
      <Text className="text-xs font-mono text-foreground-secondary leading-4" numberOfLines={3} selectable>
        {String(result.content || "")}
      </Text>
    </View>
  );
}

export function UserBubble({ content }: { content: string }) {
  return (
    <View className="bg-foreground/5 border border-border rounded-xl p-3.5 mb-3.5 self-end max-w-[85%]">
      <Text className="text-[9px] font-mono font-bold text-foreground-secondary mb-2 tracking-widest">YOU</Text>
      <Text className="text-foreground text-sm leading-5" selectable>{content}</Text>
    </View>
  );
}

export function AssistantBubble({
  textContent,
  thinkingContent,
  label = "AGENT",
  toolCalls,
  isStreaming,
}: {
  textContent?: string;
  thinkingContent?: string;
  label?: string;
  toolCalls?: { name: string }[];
  isStreaming?: boolean;
}) {
  return (
    <View className="bg-card border border-border rounded-xl p-3.5 mb-3.5 self-start w-full max-w-[92%]">
      <Text className="text-[9px] font-mono font-bold text-foreground-secondary mb-2 tracking-widest">
        {label}
      </Text>
      {thinkingContent ? (
        <View className="border-l-2 border-foreground-secondary/30 pl-2.5 mb-3">
          <Text className="text-[11px] font-mono text-foreground-secondary mb-1">💭 Thinking...</Text>
          <Text className="text-[11px] font-mono text-foreground-secondary/70 leading-[18px]">
            {thinkingContent}
          </Text>
        </View>
      ) : null}
      {textContent ? <MarkdownRenderer content={textContent} /> : null}
      {toolCalls && toolCalls.length > 0 ? (
        <View className="mt-2 pt-2 border-t border-border">
          {toolCalls.map((call, idx) => (
            <View key={idx} className="flex-row items-center my-1">
              <ActivityIndicator size="small" color={theme.colors.status.running} style={{ marginRight: 8 }} />
              <Text className="text-[11px] font-mono text-foreground-secondary">
                Running {call.name}…
              </Text>
            </View>
          ))}
        </View>
      ) : null}
      {isStreaming && !textContent && !thinkingContent && (!toolCalls || toolCalls.length === 0) ? (
        <ActivityIndicator size="small" color={theme.colors.text.muted} />
      ) : null}
    </View>
  );
}

/** Renders the agent message bubbles for the chat FlatList. */
export function MessageBubble({ item }: { item: AgentMessage }) {
  if (item.role === "user") {
    return <UserBubble content={item.content} />;
  }

  if (item.role === "toolResult") {
    return (
      <View className="w-full max-w-[92%] mb-3.5 self-start">
        {item.results.map((res, i) => (
          <ToolResultItem key={i} result={res} />
        ))}
      </View>
    );
  }

  const content = item.content as Array<{ type: string; text?: string }>;
  const textContent = (content ?? [])
    .filter((c) => c.type === "text")
    .map((c) => c.text ?? "")
    .join("\n\n");
  const thinkingContent = (content ?? [])
    .filter((c) => c.type === "thinking")
    .map((c) => c.text ?? "")
    .join("\n\n");

  return <AssistantBubble textContent={textContent} thinkingContent={thinkingContent} />;
}
