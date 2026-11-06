// The transcript surface, ported from console-ui's `chat/transcript_view.rs`:
// the message list plus the streaming lifecycle, grouped run activity,
// per-turn change chips, prompt jump popup, and empty state.
//
// This file is the render layer only. The data model (message types,
// run-activity and turn-change grouping) lives in `transcript_model.ts`, the
// streaming state in `transcript_state.ts`, and the prompt-jump popup in
// `prompt_jump.tsx`.
//
// Deviations from the Rust source, until their dependencies are ported:
// - No virtualized list and no scroll API in nsh yet: every visible row
//   renders, the follow-tail / scroll-anchor / scroll-restore logic has
//   nothing to attach to, and the scroll-to-bottom arrow is dropped. The
//   auto-load-older trigger (fired when scrolled near the top) has no scroll
//   events to watch either — the host still drives pagination through
//   `setPagination` / `onLoadOlder` once a scroll surface exists.
// - The selection machinery (selectable text, the Copy/Cite popup over a
//   selection) is a GPUI render-layer concern and is dropped.
// - The markdown-view and presentation caches are dropped: nsh re-renders
//   only what a signal change touches, and there is no virtualization to
//   remount rows against.

import { Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "../common/icons";
import { WorkingIndicator } from "./working_indicator";
import { UserMessageBubble } from "./message_user";
import { AssistantMessageBubble } from "./message_assistant";
import { ToolCalls } from "./toolcalls/toolcalls";
import { VisualCard } from "./visual/visual";
import type { VisualArtifact } from "./visual/types";
import { PromptJump } from "./prompt_jump";
import {
  groupTurnChanges,
  isHiddenToolTransport,
  toolActivityForUser,
  turnChangesForAssistant,
  type AgentMessage,
  type AssistantMessage,
  type UserMessage,
} from "./transcript_model";
import { createTranscriptState, type TranscriptState } from "./transcript_state";

// Re-exported for the tests and fixtures beside this file, which import the
// transcript's public surface from here.
export * from "./transcript_model";
export { createTranscriptState, type TranscriptState } from "./transcript_state";

/// The chat transcript: the message rows, the trailing working indicator
/// while a run streams, the empty state, and the floating jump-to-prompt
/// pill. Rows render full-width; run activity and per-turn change chips
/// attach to their owning rows.
export function TranscriptView(props: {
  state: TranscriptState;
  onFork?: (messageId: string) => void;
  onOpenChange?: (path: string, diffText: string | null) => void;
  onPreviewImage?: (uri: string) => void;
  /// Fetches the chat's full prompt list when the popup opens stale.
  onRequestPrompts?: () => void;
  /// Jumps to a prompt, given as its position counted from the newest
  /// (0 = newest).
  onJumpToPrompt?: (fromEnd: number) => void;
}) {
  const state = props.state;

  // Whether the last user's run contains any tool call: while it does, the
  // run's activity block is the live element and no trailing working
  // indicator row is needed.
  const hasActiveToolActivity = (): boolean => {
    const list = state.messages();
    const userIndex = list.map((m) => m.type === "user").lastIndexOf(true);
    if (userIndex < 0) return false;
    return list
      .slice(userIndex + 1)
      .some(
        (message) =>
          message.type === "assistant" &&
          message.content.some((part) => part.type === "toolCall"),
      );
  };

  const turns = () => groupTurnChanges(state.sessionChanges());

  const userRow = (message: UserMessage, index: number): JSX.Element => {
    const expandKey = message.id ?? `message-${index}`;
    const activity = toolActivityForUser(
      state.messages(),
      index,
      state.isStreaming(),
      state.streamingStartedAt(),
    );
    // Visual artifacts render as their own item after the run block, so
    // folding the run never hides the card.
    const visuals: { callId: string; visual: VisualArtifact }[] = activity
      ? activity.events.flatMap((event) => {
          if (event.type !== "toolCall" || event.entry.result == null) return [];
          const content = event.entry.result.content;
          if (
            content != null &&
            typeof content === "object" &&
            !Array.isArray(content) &&
            "visual" in (content as Record<string, unknown>)
          ) {
            return [
              {
                callId: event.entry.call.id,
                visual: (content as Record<string, unknown>).visual as VisualArtifact,
              },
            ];
          }
          return [];
        })
      : [];
    return (
      <View class="w-full flex-col gap-1.5">
        <UserMessageBubble
          content={message.content}
          attachments={message.attachments ?? undefined}
          annotations={message.annotations ?? undefined}
          contextFiles={message.contextFiles ?? undefined}
          createdAt={message.createdAt}
          expanded={() => state.userMessageExpanded(expandKey)}
          onToggleExpand={() => state.toggleUserMessageExpanded(expandKey)}
          onPreviewImage={props.onPreviewImage}
        />
        {activity ? (
          <ToolCalls
            runId={`run-${index}`}
            events={activity.events}
            working={() => activity.working}
            startedAt={activity.startedAt}
            elapsedSeconds={activity.elapsed}
            cwd={state.sessionCwd()}
          />
        ) : null}
        {visuals.map(({ visual }) => (
          <VisualCard artifact={visual} />
        ))}
      </View>
    );
  };

  const assistantRow = (message: AssistantMessage, index: number): JSX.Element => {
    const turn = turnChangesForAssistant(state.messages(), index, turns());
    const turnId = turn?.userMessageId ?? "";
    return (
      <AssistantMessageBubble
        contentParts={message.content}
        isStreaming={() => state.isStreaming() && index + 1 === state.messages().length}
        createdAt={message.createdAt}
        tokPerSec={message.tokPerSec}
        forkMessageId={message.id}
        onFork={props.onFork}
        thinkingExpanded={() => state.thinkingExpanded()}
        onThinkingToggle={(id, expanded) => state.toggleThinking(id, expanded)}
        fileChanges={turn}
        changesExpanded={() =>
          turnId !== "" && state.expandedTurnChanges().has(turnId)
        }
        onToggleChanges={() => state.toggleTurnChanges(turnId)}
        onOpenChange={props.onOpenChange}
        onPreviewImage={props.onPreviewImage}
      />
    );
  };

  const rows = (): JSX.Element[] => {
    const out: JSX.Element[] = [];
    state.messages().forEach((message, index) => {
      if (isHiddenToolTransport(message)) return;
      out.push(
        <View class="w-full flex justify-center px-5 py-1">
          <View class="w-full max-w-[768px]">
            {message.type === "user"
              ? userRow(message, index)
              : message.type === "assistant"
                ? assistantRow(message, index)
                : null}
          </View>
        </View>,
      );
    });
    return out;
  };

  const workingRow = (): JSX.Element | null => {
    if (!state.isStreaming() || hasActiveToolActivity()) return null;
    const startedAt =
      state.streamingStartedAt() ?? Math.floor(Date.now() / 1000);
    return (
      <View class="w-full flex justify-center px-5 py-1">
        <View class="w-full max-w-[768px]">
          <WorkingIndicator startedAt={startedAt} />
        </View>
      </View>
    );
  };

  // The branch reads the signals inside JSX so it re-evaluates when the
  // first message arrives or streaming starts (the component itself runs
  // once).
  return (
    <View class="h-full w-full">
      {state.messages().length === 0 && !state.isStreaming() ? (
        <View class="h-full w-full flex-col items-center justify-center gap-2">
          <Icon name="Bot" size={32} color={token("muted-foreground")} />
          <Text class="text-[14px] text-muted-foreground">
            Start a conversation
          </Text>
        </View>
      ) : (
        <View class="group relative h-full w-full flex-col">
          <View class="h-full w-full flex-col pb-11">{rows()}</View>
          {workingRow()}
          <PromptJump
            state={state}
            onRequestPrompts={props.onRequestPrompts}
            onJumpToPrompt={props.onJumpToPrompt}
          />
        </View>
      )}
    </View>
  );
}
