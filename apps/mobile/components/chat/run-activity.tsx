import React, { memo, useEffect, useMemo, useState } from "react";
import { View, Text, Pressable, ActivityIndicator } from "react-native";
import { ChevronRight, ChevronDown, Sparkles } from "lucide-react-native";
import type { ActivityEvent, RunActivityState } from "../../types/chat";
import type { ToolCall, ToolResult } from "@console/types";
import { ToolCallBlock } from "./tool-call-block";
import { MarkdownRenderer } from "../common/markdown-renderer";
import { formatDuration } from "../../utils";
import { theme } from "../../styles/theme";

interface RunActivityProps {
  activity: RunActivityState;
  /** True only for the latest run while the session is actively running. */
  running: boolean;
}

type RenderGroup =
  | { kind: "text"; id: string; text: string }
  | { kind: "thinking"; id: string; text: string }
  | { kind: "tools"; id: string; calls: ToolCall[]; results: ToolResult[] };

/**
 * Group consecutive events: text/thinking events render individually,
 * consecutive tool call events of the same tool name render as a single
 * ToolCallBlock.
 */
function groupEvents(events: ActivityEvent[]): RenderGroup[] {
  const groups: RenderGroup[] = [];
  for (const event of events) {
    if (event.type === "text") {
      groups.push({ kind: "text", id: event.id, text: event.text });
    } else if (event.type === "thinking") {
      groups.push({ kind: "thinking", id: event.id, text: event.text });
    } else {
      // toolCall event
      const last = groups[groups.length - 1];
      if (
        last?.kind === "tools" &&
        last.calls.length > 0 &&
        last.calls[last.calls.length - 1]!.name === event.call.name
      ) {
        last.calls.push(event.call);
        if (event.result) last.results.push(event.result);
      } else {
        groups.push({
          kind: "tools",
          id: event.call.id,
          calls: [event.call],
          results: event.result ? [event.result] : [],
        });
      }
    }
  }
  return groups;
}

const CollapsibleThinking = memo(function CollapsibleThinking({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <View className="mb-2">
      <Pressable
        onPress={() => setExpanded((v) => !v)}
        className="flex-row items-center gap-1.5 self-start py-1"
        hitSlop={8}
      >
        <Sparkles size={12} color={theme.colors.text.muted} />
        <Text className="text-xs font-semibold text-foreground-secondary">Thought</Text>
        <ChevronDown
          size={13}
          color={theme.colors.text.muted}
          style={{ transform: [{ rotate: expanded ? "0deg" : "-90deg" }] }}
        />
      </Pressable>
      {expanded && text ? (
        <View className="mt-1 border-l-2 border-white/10 pl-3">
          <Text className="text-[13px] text-foreground-secondary leading-5" selectable>
            {text}
          </Text>
        </View>
      ) : null}
    </View>
  );
});

export const RunActivity = memo(function RunActivity({
  activity,
  running,
}: RunActivityProps) {
  const [expanded, setExpanded] = useState(running);
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    setExpanded(running);
    if (!running) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [running]);

  const groups = useMemo(() => groupEvents(activity.events), [activity.events]);

  // Don't render the activity block if there are no tool calls or events
  const hasToolCalls = activity.events.some((e) => e.type === "toolCall");
  if (!hasToolCalls && !running) return null;

  const isWorking = running || activity.status === "working";
  const elapsed = isWorking && activity.startedAt ? now - activity.startedAt : activity.elapsedMs;

  const summaryLabel = isWorking
    ? `Working for ${formatDuration(elapsed)}…`
    : activity.status === "aborted"
      ? `Aborted after ${formatDuration(elapsed)}`
      : activity.status === "failed"
        ? `Failed after ${formatDuration(elapsed)}`
        : `Worked for ${formatDuration(elapsed)}`;

  return (
    <View className="border-b border-white/[0.06] pb-1 mb-1.5">
      <Pressable
        onPress={() => setExpanded((current) => !current)}
        className="flex-row items-center gap-1.5 self-start py-1.5 px-1"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
      >
        {isWorking ? <ActivityIndicator size="small" color={theme.colors.text.muted} /> : null}
        <Text className="text-xs font-medium text-foreground-secondary">
          {summaryLabel}
        </Text>
        <ChevronDown
          size={14}
          color={theme.colors.text.muted}
          style={{ transform: [{ rotate: expanded ? "0deg" : "-90deg" }] }}
        />
      </Pressable>

      {expanded && groups.length > 0 ? (
        <View className="mt-1.5 space-y-2">
          {groups.map((group) => {
            if (group.kind === "thinking") {
              return <CollapsibleThinking key={`${group.kind}-${group.id}`} text={group.text} />;
            }
            if (group.kind === "text") {
              return (
                <View key={`${group.kind}-${group.id}`} className="px-1 mb-2">
                  <MarkdownRenderer content={group.text} />
                </View>
              );
            }
            return (
              <ToolCallBlock
                key={`${group.kind}-${group.id}`}
                calls={group.calls}
                results={group.results}
              />
            );
          })}
        </View>
      ) : null}
    </View>
  );
});
