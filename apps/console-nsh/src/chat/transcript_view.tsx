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
// - The rows render through nsh's virtualized `List`: only rows near the
//   viewport exist, cells recycle, tail-following streams into view via
//   `maintainVisibleContentPosition`, and `onLoadMoreAtStart` fires the
//   host's `onLoadOlder` when the list approaches its start. The desktop's
//   scroll-to-bottom arrow is back as a floating pill shown when scrolled
//   away from the end.
// - The selection machinery (selectable text, the Copy/Cite popup over a
//   selection) is a GPUI render-layer concern and is dropped.
// - The markdown-view and presentation caches are dropped: nsh re-renders
//   only what a signal change touches, and list recycling replaces the
//   virtualization the caches served.

import { List, Text, View, token, signal } from "nsh";
import type { JSX, ListRef } from "nsh";
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
  messageId,
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
  onRequestPrompts?: () => Promise<void> | void;
  /// Jumps to a prompt, given as its position counted from the newest
  /// (0 = newest).
  onJumpToPrompt?: (fromEnd: number) => void;
  /// Fetches the page of messages before the first one (the list reached
  /// its start and `state.hasMore()` is set).
  onLoadOlder?: () => void;
}) {
  const state = props.state;

  let listRef: ListRef | undefined;
  // Whether the list rests at (or within a viewport of) the end: the
  // scroll-to-bottom pill only shows while scrolled away from it.
  const atEnd = signal(true);

  // Whether the last user's run contains any tool call: while it does, the
  // run's activity block is the live element and no trailing working
  // indicator row is needed.
  const hasActiveToolActivity = (): boolean => {
    const list = state.messages();
    const userIndex = list.map((m) => m.role === "user").lastIndexOf(true);
    if (userIndex < 0) return false;
    return list
      .slice(userIndex + 1)
      .some(
        (message) =>
          message.role === "assistant" &&
          message.content.some((part) => part.type === "toolCall"),
      );
  };

  const turns = () => groupTurnChanges(state.sessionChanges());

  const visibleMessages = (): { message: AgentMessage; index: number }[] => {
    const out: { message: AgentMessage; index: number }[] = [];
    state.messages().forEach((message, index) => {
      if (!isHiddenToolTransport(message)) out.push({ message, index });
    });
    return out;
  };

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
      ? activity.events.flatMap((event) =>
          event.type === "toolCall" && event.entry.result?.visual != null
            ? [{ callId: event.entry.call.id, visual: event.entry.result.visual }]
            : [],
        )
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

  const row = (message: AgentMessage, index: number): JSX.Element => (
    <View class="w-full flex justify-center px-5 py-1">
      <View class="w-full max-w-[768px]">
        {message.role === "user"
          ? userRow(message, index)
          : message.role === "assistant"
            ? assistantRow(message, index)
            : null}
      </View>
    </View>
  );

  const messageKey = (item: { message: AgentMessage; index: number }): string =>
    messageId(item.message) ?? `idx-${item.index}`;

  // Counted from the newest prompt, like the desktop; a prompt not paged in
  // yet cannot be reached.
  const jumpToPrompt = (fromEnd: number) => {
    const rows = visibleMessages();
    const users = rows.flatMap((item, at) => (item.message.role === "user" ? [at] : []));
    const at = users[users.length - 1 - fromEnd];
    if (at != null) listRef?.scrollToIndex({ index: at, animated: true });
    props.onJumpToPrompt?.(fromEnd);
  };

  const loadOlder = () => {
    if (!state.hasMore() || state.loadingOlder()) return;
    state.setLoadingOlder(true);
    props.onLoadOlder?.();
  };

  // The branch reads the signals inside JSX so it re-evaluates when the
  // first message arrives or streaming starts (the component itself runs
  // once).
  return (
    <View class="group relative h-full w-full">
      <List
        ref={(ref) => {
          listRef = ref;
        }}
        class="h-full w-full"
        contentClass="pb-11"
        each={visibleMessages()}
        keyExtractor={messageKey}
        maintainVisibleContentPosition={{ autoscrollToBottomThreshold: 120 }}
        onScroll={(e) =>
          atEnd.set(e.offset.y + e.size.height >= e.contentSize.height - 80)
        }
        onLoadMoreAtStart={loadOlder}
        empty={
          <View class="h-full w-full flex-col items-center justify-center gap-2">
            <Icon name="Bot" size={32} color={token("muted-foreground")} />
            <Text class="text-[14px] text-muted-foreground">
              Start a conversation
            </Text>
          </View>
        }
        footer={
          state.isStreaming() && !hasActiveToolActivity() ? (
            <View class="w-full flex justify-center px-5 py-1">
              <View class="w-full max-w-[768px]">
                <WorkingIndicator
                  startedAt={
                    state.streamingStartedAt() ?? Math.floor(Date.now() / 1000)
                  }
                />
              </View>
            </View>
          ) : null
        }
      >
        {(item, index) => row(item().message, item().index)}
      </List>
      <PromptJump
        state={state}
        onRequestPrompts={props.onRequestPrompts}
        onJumpToPrompt={jumpToPrompt}
        showScrollToBottom={() => !atEnd()}
        onScrollToBottom={() => {
          atEnd.set(true);
          listRef?.scrollToEnd({ animated: true });
        }}
      />
    </View>
  );
}
