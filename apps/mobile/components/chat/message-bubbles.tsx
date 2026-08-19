import React, { memo, useEffect, useRef, useState } from "react";
import {
  ActivityIndicator,
  Animated,
  Pressable,
  Text,
  View,
} from "react-native";
import {
  AlertTriangle,
  Check,
  ChevronDown,
  Copy,
  Sparkles,
  Wrench,
} from "lucide-react-native";
import { setStringAsync } from "expo-clipboard";
import type { AgentMessage, ToolResult } from "@console/types";
import { MarkdownRenderer } from "../common/markdown-renderer";
import { theme } from "../../styles/theme";
import { formatMessageTime } from "../../utils/time";

/** Animated three-dot typing indicator for streaming states. */
const TypingDots = memo(function TypingDots() {
  const dot1 = useRef(new Animated.Value(0.3)).current;
  const dot2 = useRef(new Animated.Value(0.3)).current;
  const dot3 = useRef(new Animated.Value(0.3)).current;

  useEffect(() => {
    const animate = (value: Animated.Value, delay: number) => {
      const loop = Animated.loop(
        Animated.sequence([
          Animated.timing(value, {
            toValue: 1,
            duration: 500,
            delay,
            useNativeDriver: true,
          }),
          Animated.timing(value, {
            toValue: 0.3,
            duration: 500,
            useNativeDriver: true,
          }),
        ]),
      );
      loop.start();
      return loop;
    };
    const loops = [dot1, dot2, dot3].map((v, i) => animate(v, i * 160));
    return () => loops.forEach((l) => l.stop());
  }, [dot1, dot2, dot3]);

  const dotStyle = (value: Animated.Value) => ({
    opacity: value,
  });

  return (
    <View className="flex-row items-center gap-1.5 py-2">
      {[dot1, dot2, dot3].map((v, i) => (
        <Animated.View
          key={i}
          className="w-1.5 h-1.5 rounded-full"
          style={[{ backgroundColor: theme.colors.text.muted }, dotStyle(v)]}
        />
      ))}
    </View>
  );
});

/** Collapsible "Thought" block. Expands while streaming so the user sees live reasoning. */
const ThinkingBlock = memo(function ThinkingBlock({
  text,
  isStreaming,
}: {
  text: string;
  isStreaming?: boolean;
}) {
  const [expanded, setExpanded] = useState(Boolean(isStreaming));

  return (
    <View className="mb-3">
      <Pressable
        onPress={() => setExpanded((v) => !v)}
        className="flex-row items-center gap-1.5 self-start"
        hitSlop={8}
      >
        <Sparkles size={13} color={theme.colors.text.muted} />
        <Text className="text-xs font-semibold text-foreground-secondary">
          {isStreaming ? "Thinking…" : "Thought"}
        </Text>
        <ChevronDown
          size={14}
          color={theme.colors.text.muted}
          style={{ transform: [{ rotate: expanded ? "0deg" : "-90deg" }] }}
        />
      </Pressable>
      {expanded && text ? (
        <View className="mt-2 border-l-2 pl-3.5" style={{ borderColor: theme.colors.borderSubtle }}>
          <Text className="text-[13px] text-foreground-secondary leading-5" selectable>
            {text}
          </Text>
        </View>
      ) : null}
    </View>
  );
});

/** Compact collapsible tool-activity row (running / done / failed). */
export const ToolActivityRow = memo(function ToolActivityRow({
  name,
  isRunning,
  isError,
  detail,
}: {
  name: string;
  isRunning?: boolean;
  isError?: boolean;
  detail?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const statusColor = isError
    ? theme.colors.status.attention
    : isRunning
      ? theme.colors.status.running
      : theme.colors.status.ready;
  const statusBg = isError
    ? theme.colors.status.attentionBg
    : isRunning
      ? theme.colors.status.runningBg
      : theme.colors.status.readyBg;

  return (
    <Pressable
      onPress={() => setExpanded((v) => !v)}
      className="mb-1.5 rounded-xl overflow-hidden"
      style={{ backgroundColor: "rgba(255,255,255,0.04)" }}
      hitSlop={4}
    >
      <View className="flex-row items-center gap-2.5 px-3 py-2">
        <View
          className="w-7 h-7 rounded-lg items-center justify-center"
          style={{ backgroundColor: statusBg }}
        >
          {isRunning ? (
            <ActivityIndicator size="small" color={statusColor} />
          ) : isError ? (
            <AlertTriangle size={14} color={statusColor} />
          ) : (
            <Wrench size={13} color={statusColor} />
          )}
        </View>
        <Text className="flex-1 text-[13px] font-mono text-foreground" numberOfLines={1}>
          {name}
        </Text>
        {isRunning ? (
          <Text className="text-[11px] font-semibold text-foreground-secondary">{detail ?? "Running"}</Text>
        ) : (
          <View className="flex-row items-center gap-1.5">
            <Text
              className="text-[10px] font-bold font-mono tracking-wide"
              style={{ color: statusColor }}
            >
              {isError ? "FAILED" : "DONE"}
            </Text>
            <ChevronDown
              size={12}
              color={theme.colors.text.muted}
              style={{ transform: [{ rotate: expanded ? "0deg" : "-90deg" }] }}
            />
          </View>
        )}
      </View>
      {expanded && detail ? (
        <View className="px-3.5 pb-2.5 pt-1" style={{ backgroundColor: "rgba(255,255,255,0.02)" }}>
          <Text className="text-[12px] font-mono text-foreground-secondary leading-4" selectable>
            {detail}
          </Text>
        </View>
      ) : null}
    </Pressable>
  );
});

export const ToolResultItem = memo(function ToolResultItem({ result }: { result: ToolResult }) {
  const isError = result.isError;
  const detail =
    typeof result.content === "string"
      ? result.content
      : result.content === null || result.content === undefined
        ? ""
        : JSON.stringify(result.content, null, 2);
  return (
    <ToolActivityRow name={result.toolName ?? "tool"} isError={isError} detail={detail} />
  );
});

const MessageCopyButton = memo(function MessageCopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (!text) return;
    await setStringAsync(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Pressable
      onPress={handleCopy}
      hitSlop={8}
      className="flex-row items-center p-1 rounded-md"
      style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
    >
      {copied ? (
        <Check size={12} color={theme.colors.status.ready} />
      ) : (
        <Copy size={12} color={theme.colors.text.muted} />
      )}
    </Pressable>
  );
});

export const UserBubble = memo(function UserBubble({
  content,
  createdAt,
}: {
  content: string;
  createdAt?: number;
}) {
  return (
    <View className="items-end mb-4">
      <View
        className="max-w-[85%] px-4 py-2.5 rounded-[20px] rounded-br-md"
        style={{ backgroundColor: theme.colors.surfaceElevated }}
      >
        <Text className="text-foreground text-[15px] leading-[22px]" selectable>
          {content}
        </Text>
      </View>
      <View className="flex-row items-center gap-1.5 mt-1 mr-0.5">
        {createdAt ? (
          <Text className="text-[11px] text-foreground-secondary/70">
            {formatMessageTime(createdAt)}
          </Text>
        ) : null}
        <MessageCopyButton text={content} />
      </View>
    </View>
  );
});

export const AssistantBubble = memo(function AssistantBubble({
  textContent,
  thinkingContent,
  isStreaming,
  createdAt,
}: {
  textContent?: string;
  thinkingContent?: string;
  isStreaming?: boolean;
  createdAt?: number;
}) {
  const hasContent = Boolean(textContent) || Boolean(thinkingContent);
  const showTyping = isStreaming && !hasContent;

  const copyableText = [thinkingContent ? `Thought:\n${thinkingContent}` : "", textContent ?? ""]
    .filter(Boolean)
    .join("\n\n");

  return (
    <View className="mb-4">
      {showTyping ? <TypingDots /> : null}

      {thinkingContent ? <ThinkingBlock text={thinkingContent} isStreaming={isStreaming} /> : null}

      {textContent ? <MarkdownRenderer content={textContent} /> : null}

      {isStreaming && textContent ? (
        <View className="mt-1 flex-row items-center gap-1.5">
          <ActivityIndicator size="small" color={theme.colors.text.muted} />
        </View>
      ) : null}

      {!isStreaming && !showTyping && !textContent && !thinkingContent ? (
        <View className="flex-row items-center gap-1.5">
          <Check size={13} color={theme.colors.status.ready} />
          <Text className="text-xs text-foreground-secondary">Done</Text>
        </View>
      ) : null}

      {/* Time and Copy Button rendered at the end of the assistant message */}
      {!isStreaming && (createdAt || copyableText) ? (
        <View className="flex-row items-center gap-1.5 mt-1.5 ml-0.5">
          {createdAt ? (
            <Text className="text-[11px] text-foreground-secondary/70">
              {formatMessageTime(createdAt)}
            </Text>
          ) : null}
          {copyableText ? <MessageCopyButton text={copyableText} /> : null}
        </View>
      ) : null}
    </View>
  );
});

/** Renders the agent message bubbles for the chat FlashList. */
export const MessageBubble = memo(function MessageBubble({
  item,
  isStreaming,
}: {
  item: AgentMessage;
  isStreaming?: boolean;
}) {
  if (item.role === "user") {
    return <UserBubble content={item.content} createdAt={item.createdAt} />;
  }

  if (item.role === "toolResult") {
    return null;
  }

  const content = item.content as Array<{ type: string; text?: string; call?: { name: string } }>;
  const hasToolCalls = (content ?? []).some((c) => c.type === "toolCall");

  // If this turn has tool calls, suppress text here — it is shown in RunActivity
  if (hasToolCalls && !isStreaming) {
    return null;
  }

  const textContent = (content ?? [])
    .filter((c) => c.type === "text")
    .map((c) => c.text ?? "")
    .join("\n\n");
  const thinkingContent = (content ?? [])
    .filter((c) => c.type === "thinking")
    .map((c) => c.text ?? "")
    .join("\n\n");

  return (
    <AssistantBubble
      textContent={textContent}
      thinkingContent={thinkingContent}
      isStreaming={isStreaming}
      createdAt={item.createdAt}
    />
  );
});