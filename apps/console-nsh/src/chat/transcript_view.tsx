// The transcript surface, ported from console-ui's `chat/transcript_view.rs`:
// the message list plus the streaming lifecycle, grouped run activity,
// per-turn change chips, prompt jump popup, and empty state. The desktop's
// state lives on a GPUI entity; here `createTranscriptState()` returns the
// same state as nsh signals plus the mutation methods the host app calls
// (`set_messages`, `push_message`, `append_assistant_text`, …), and
// `TranscriptView` renders it.
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
// - `ToolCallsState` lived on the entity so virtualized rows kept their
//   disclosure state; rows never unmount here, and `ToolCalls` owns its
//   state locally.
// - The floating controls pill always shows instead of hover-only (nsh has
//   no group hover).

import { Text, View, signal, token } from "nsh";
import type { JSX } from "nsh";
import { Popover } from "nsh/base";
import { Icon } from "../common/icons";
import { WorkingIndicator } from "./working_indicator";
import { UserMessageBubble } from "./message_user";
import { AssistantMessageBubble, type AssistantContentPart } from "./message_assistant";
import type { BrowserElementAnnotation, ImageAttachment } from "./message_user";
import { ToolCalls, elapsedSeconds as elapsedSecondsBetween, type ActivityEvent, type ToolCall, type ToolResult } from "./toolcalls";
import { VisualCard } from "./visual/visual";
import type { VisualArtifact } from "./visual/types";
import {
  changesSummaryFrom,
  changesSummaryIsEmpty,
  turnChangeFrom,
  type TurnChanges,
  type ChangesSummary,
} from "./change_chips";
import { formatTimeAgo } from "../utils/time";

// ─── Message model (console-core's `AgentMessage`) ──────────────────────────

export interface UserMessage {
  type: "user";
  id?: string | null;
  content: string;
  attachments?: ImageAttachment[] | null;
  annotations?: BrowserElementAnnotation[] | null;
  contextFiles?: string[] | null;
  createdAt?: number | null;
}

export interface AssistantMessage {
  type: "assistant";
  id?: string | null;
  content: AssistantContentPart[];
  stopReason?: string | null;
  createdAt?: number | null;
  tokPerSec?: number | null;
}

export interface ToolResultMessage {
  type: "toolResult";
  results: ToolResult[];
  createdAt?: number | null;
}

export type AgentMessage = UserMessage | AssistantMessage | ToolResultMessage;

/// console-core's `SessionFileChange` wire shape.
export interface SessionFileChange {
  path: string;
  status: string;
  additions: number;
  deletions: number;
  diffText: string | null;
  userMessageId: string;
}

/// console-core's `SessionPrompt`.
export interface SessionPrompt {
  id: string;
  prompt: string;
  createdAt: number;
}

/// Is this message transport-only after run activity takes ownership of its
/// visual output? Tool-result messages are persistence transport, and an
/// assistant message with tool calls is fully represented by its run block.
export function isHiddenToolTransport(message: AgentMessage): boolean {
  if (message.type === "toolResult") return true;
  if (message.type === "assistant") {
    return message.content.some((part) => part.type === "toolCall");
  }
  return false;
}

// ─── Run activity (`tool_activity_for_user`) ────────────────────────────────

export interface RunActivity {
  events: ActivityEvent[];
  startedAt: number | null;
  elapsed: number;
  working: boolean;
}

function normalizeUnixTimestamp(timestamp: number): number {
  return timestamp > 10_000_000_000 ? Math.floor(timestamp / 1000) : timestamp;
}

function maxTimestamp(a: number | null, b: number | null): number | null {
  if (a == null) return b;
  if (b == null) return a;
  return Math.max(a, b);
}

/// Collect the tool calls, thinking, and progress text belonging to the
/// assistant run after a user message, in chronological content order. The
/// desktop client uses the same boundary: a new user message starts the next
/// run, and thinking/text between tool calls stay in the timeline. Text and
/// thinking only enter the timeline when that same turn contains a tool
/// call; a final answer with none renders as a standalone bubble.
export function toolActivityForUser(
  messages: AgentMessage[],
  userIndex: number,
  isStreaming: boolean,
  streamingStartedAt: number | null,
): RunActivity | undefined {
  const events: ActivityEvent[] = [];
  const results = new Map<string, ToolResult>();
  let endedAt: number | null = null;

  for (const message of messages.slice(userIndex + 1)) {
    if (message.type === "user") break;
    if (message.type === "assistant") {
      endedAt = maxTimestamp(
        endedAt,
        message.createdAt != null
          ? normalizeUnixTimestamp(message.createdAt)
          : null,
      );
      const hasToolCall = message.content.some((part) => part.type === "toolCall");
      if (!hasToolCall) continue;
      for (const part of message.content) {
        if (part.type === "thinking" && part.text.trim().length > 0) {
          events.push({
            type: "thinking",
            id: `run-thinking-${userIndex}-${events.length}`,
            text: part.text,
          });
        } else if (part.type === "text" && part.text.trim().length > 0) {
          events.push({
            type: "text",
            id: `run-text-${userIndex}-${events.length}`,
            text: part.text,
          });
        } else if (part.type === "toolCall") {
          events.push({ type: "toolCall", entry: { call: part.call, result: null } });
        }
      }
    } else {
      endedAt = maxTimestamp(
        endedAt,
        message.createdAt != null
          ? normalizeUnixTimestamp(message.createdAt)
          : null,
      );
      for (const result of message.results) {
        results.set(result.toolCallId, result);
      }
    }
  }

  const hasToolCalls = events.some((event) => event.type === "toolCall");
  if (!hasToolCalls) return undefined;

  const joined = events.map((event) =>
    event.type === "toolCall" && results.has(event.entry.call.id)
      ? {
          ...event,
          entry: { ...event.entry, result: results.get(event.entry.call.id)! },
        }
      : event,
  );

  const user = messages[userIndex];
  const userStartedAt =
    user?.type === "user" && user.createdAt != null
      ? normalizeUnixTimestamp(user.createdAt)
      : null;
  const latestUser = messages.map((m) => m.type === "user").lastIndexOf(true);
  const working = isStreaming && latestUser === userIndex;
  const startedAt = working ? (streamingStartedAt ?? userStartedAt) : userStartedAt;
  const elapsed = elapsedSecondsBetween(startedAt, endedAt);
  return { events: joined, startedAt, elapsed, working };
}

// ─── Turn changes (`group_turn_changes`) ────────────────────────────────────

/// Group the session's file changes by the user message that started their
/// turn. The server sends rows most recently updated first; each turn's
/// files are reversed so chips read in the order the agent touched them.
/// Rows with no user message id (nothing links them to a turn) are dropped.
export function groupTurnChanges(
  changes: SessionFileChange[],
): Map<string, TurnChanges> {
  const grouped = new Map<string, TurnChanges>();
  for (const change of [...changes].reverse()) {
    if (change.userMessageId === "") continue;
    const turn = grouped.get(change.userMessageId) ?? {
      userMessageId: change.userMessageId,
      files: [],
    };
    turn.files.push(turnChangeFrom(change));
    grouped.set(change.userMessageId, turn);
  }
  return grouped;
}

/// File changes of the turn a settled assistant message closes. Only the
/// turn's last visible assistant message carries them: a turn is one user
/// message and everything the agent did until the next one.
export function turnChangesForAssistant(
  messages: AgentMessage[],
  index: number,
  turns: Map<string, TurnChanges>,
): TurnChanges | undefined {
  if (turns.size === 0) return undefined;
  const message = messages[index];
  if (message?.type !== "assistant") return undefined;
  if (isHiddenToolTransport(message)) return undefined;
  const closesTurn = (): boolean => {
    for (const later of messages.slice(index + 1)) {
      if (later.type === "user") break;
      if (later.type === "assistant" && !isHiddenToolTransport(later)) {
        return false;
      }
    }
    return true;
  };
  if (!closesTurn()) return undefined;
  let userId: string | undefined;
  for (let i = index - 1; i >= 0; i--) {
    const earlier = messages[i];
    if (earlier.type === "user" && earlier.id != null) {
      userId = earlier.id;
      break;
    }
  }
  if (userId == null) return undefined;
  return turns.get(userId);
}

// ─── State (`TranscriptView` entity) ────────────────────────────────────────

/// The transcript's mutable state and streaming API, held as nsh signals so
/// the rendered rows track them. Mirrors the desktop entity's public
/// mutation surface; scroll and cache internals are not ported (see the
/// header note).
export function createTranscriptState() {
  const messages = signal<AgentMessage[]>([]);
  const isStreaming = signal(false);
  const streamingStartedAt = signal<number | null>(null);
  const thinkingExpanded = signal(new Set<string>());
  const expandedUserMessages = signal(new Set<string>());
  const expandedTurnChanges = signal(new Set<string>());
  const sessionChanges = signal<SessionFileChange[]>([]);
  const hasMore = signal(false);
  const nextCursor = signal<number | null>(null);
  const loadingOlder = signal(false);
  const sessionCwd = signal<string | null>(null);
  const prompts = signal<SessionPrompt[]>([]);
  const promptsOpen = signal(false);
  const promptsStale = signal(true);

  const lastAssistant = (list: AgentMessage[]): AssistantMessage | undefined => {
    const last = list[list.length - 1];
    return last?.type === "assistant" ? last : undefined;
  };

  const upsertToolCall = (list: AgentMessage[], call: ToolCall): void => {
    const existing = lastAssistant(list);
    if (existing) {
      const part = existing.content.find(
        (p) => p.type === "toolCall" && p.call.id === call.id,
      );
      if (part && part.type === "toolCall") {
        part.call = call;
      } else {
        existing.content.push({ type: "toolCall", call });
      }
    } else {
      list.push({
        type: "assistant",
        content: [{ type: "toolCall", call }],
        createdAt: Math.floor(Date.now() / 1000),
      });
    }
  };

  return {
    messages,
    isStreaming,
    streamingStartedAt,
    thinkingExpanded,
    expandedUserMessages,
    expandedTurnChanges,
    sessionChanges,
    hasMore,
    nextCursor,
    loadingOlder,
    sessionCwd,
    prompts,
    promptsOpen,
    promptsStale,

    /// Replace the whole message list (a loaded or switched session).
    setMessages(list: AgentMessage[]) {
      prompts.set([]);
      promptsStale.set(true);
      thinkingExpanded.set(new Set());
      expandedUserMessages.set(new Set());
      expandedTurnChanges.set(new Set());
      messages.set(list);
      isStreaming.set(false);
      streamingStartedAt.set(null);
    },

    /// Insert a page of older messages at the front, skipping ones already
    /// present (matched like the desktop's `messages_match`: same user
    /// content, same assistant id, or same first tool-call id).
    prependMessages(older: AgentMessage[], hasMoreNext: boolean, cursor: number | null) {
      const fresh = older.filter(
        (old) => !messages().some((existing) => messagesMatch(old, existing)),
      );
      hasMore.set(hasMoreNext);
      nextCursor.set(cursor);
      if (fresh.length === 0) {
        loadingOlder.set(false);
        return;
      }
      messages.set([...fresh, ...messages()]);
      thinkingExpanded.set(new Set());
      expandedUserMessages.set(new Set());
      expandedTurnChanges.set(new Set());
      loadingOlder.set(false);
    },

    pushMessage(message: AgentMessage) {
      if (message.type === "user") promptsStale.set(true);
      messages.set([...messages(), message]);
      // A new prompt means the user wants to see its response: the tail
      // follows (automatic here — there is no scroll position to hold).
    },

    beginStreaming() {
      isStreaming.set(true);
      streamingStartedAt.set(Math.floor(Date.now() / 1000));
    },

    /// Re-engage the trailing working indicator with the run's real start
    /// time, so the elapsed counter continues instead of restarting.
    resumeStreaming(startedAt: number) {
      isStreaming.set(true);
      streamingStartedAt.set(startedAt);
    },

    finishStreaming() {
      isStreaming.set(false);
      streamingStartedAt.set(null);
    },

    appendAssistantText(text: string) {
      const list = [...messages()];
      const existing = lastAssistant(list);
      if (existing) {
        const lastPart = existing.content[existing.content.length - 1];
        if (lastPart?.type === "text") {
          existing.content = [
            ...existing.content.slice(0, -1),
            { ...lastPart, text: lastPart.text + text },
          ];
        } else {
          existing.content = [...existing.content, { type: "text", text }];
        }
        messages.set(list);
      } else {
        messages.set([
          ...list,
          {
            type: "assistant",
            content: [{ type: "text", text }],
            createdAt: Math.floor(Date.now() / 1000),
          },
        ]);
      }
    },

    appendAssistantThinking(text: string) {
      const list = [...messages()];
      const existing = lastAssistant(list);
      if (existing) {
        const lastPart = existing.content[existing.content.length - 1];
        if (lastPart?.type === "thinking") {
          existing.content = [
            ...existing.content.slice(0, -1),
            { ...lastPart, text: lastPart.text + text },
          ];
        } else {
          existing.content = [...existing.content, { type: "thinking", text }];
        }
        messages.set(list);
      } else {
        messages.set([
          ...list,
          { type: "assistant", content: [{ type: "thinking", text }] },
        ]);
      }
    },

    /// Replace the streaming assistant message with its persisted form.
    finalizeAssistantMessage(message: Partial<AssistantMessage> & { content: AssistantContentPart[] }) {
      const list = [...messages()];
      const existing = lastAssistant(list);
      if (existing) {
        messages.set([
          ...list.slice(0, -1),
          {
            ...existing,
            id: message.id ?? existing.id,
            content: message.content,
            stopReason: message.stopReason ?? existing.stopReason,
            createdAt: message.createdAt ?? existing.createdAt,
            tokPerSec: message.tokPerSec ?? existing.tokPerSec,
          },
        ]);
      } else {
        messages.set([...list, { type: "assistant", ...message } as AssistantMessage]);
      }
    },

    upsertAssistantToolCall(call: ToolCall) {
      const list = [...messages()];
      upsertToolCall(list, call);
      messages.set(list);
    },

    upsertAssistantToolCalls(calls: ToolCall[]) {
      if (calls.length === 0) return;
      const list = [...messages()];
      for (const call of calls) upsertToolCall(list, call);
      messages.set(list);
    },

    appendToolResults(results: ToolResult[]) {
      const existingIds = new Set(
        messages().flatMap((message) =>
          message.type === "toolResult"
            ? message.results.map((result) => result.toolCallId)
            : [],
        ),
      );
      const fresh = results.filter((r) => !existingIds.has(r.toolCallId));
      if (fresh.length === 0) return;
      messages.set([
        ...messages(),
        { type: "toolResult", results: fresh, createdAt: Math.floor(Date.now() / 1000) },
      ]);
    },

    /// Copy server-assigned ids onto this transcript's user messages that
    /// still lack one (the optimistic prompt of a live run), paired from the
    /// tail with the canonical page.
    adoptUserMessageIds(canonical: AgentMessage[]) {
      const canonicalIds = [...canonical]
        .reverse()
        .filter((message): message is UserMessage => message.type === "user")
        .map((message) => message.id);
      const list = [...messages()];
      let changed = false;
      let at = 0;
      for (let i = list.length - 1; i >= 0; i--) {
        const message = list[i];
        if (message.type !== "user") continue;
        const canonicalId = canonicalIds[at++];
        if (canonicalId == null) break;
        if (message.id == null && canonicalId != null) {
          list[i] = { ...message, id: canonicalId };
          changed = true;
        }
      }
      if (changed) messages.set(list);
    },

    /// Replace the session's recorded file changes.
    setSessionChanges(changes: SessionFileChange[]) {
      sessionChanges.set(changes);
    },

    /// Whole-session change totals for the pinned summary pill above the
    /// composer. `undefined` when the session has not touched any file.
    changesSummary(): ChangesSummary | undefined {
      const summary = changesSummaryFrom(sessionChanges());
      return changesSummaryIsEmpty(summary) ? undefined : summary;
    },

    setPagination(more: boolean, cursor: number | null) {
      hasMore.set(more);
      nextCursor.set(cursor);
      loadingOlder.set(false);
    },

    setLoadingOlder(loading: boolean) {
      loadingOlder.set(loading);
    },

    setSessionCwd(cwd: string | null) {
      sessionCwd.set(cwd == null || cwd === "" ? null : cwd);
    },

    setPrompts(list: SessionPrompt[]) {
      prompts.set(list);
    },

    userMessageExpanded(key: string): boolean {
      return expandedUserMessages().has(key);
    },

    toggleUserMessageExpanded(key: string) {
      const next = new Set(expandedUserMessages());
      if (next.has(key)) next.delete(key);
      else next.add(key);
      expandedUserMessages.set(next);
    },

    toggleThinking(thinkingId: string, expanded: boolean) {
      const next = new Set(thinkingExpanded());
      if (expanded) next.add(thinkingId);
      else next.delete(thinkingId);
      thinkingExpanded.set(next);
    },

    toggleTurnChanges(turnId: string) {
      const next = new Set(expandedTurnChanges());
      if (next.has(turnId)) next.delete(turnId);
      else next.add(turnId);
      expandedTurnChanges.set(next);
    },
  };
}

export type TranscriptState = ReturnType<typeof createTranscriptState>;

/// Whether two messages are the same one seen twice (a page overlap).
function messagesMatch(a: AgentMessage, b: AgentMessage): boolean {
  if (a.type === "user" && b.type === "user") {
    return (
      a.content === b.content &&
      (a.createdAt == null || b.createdAt == null || a.createdAt === b.createdAt)
    );
  }
  if (a.type === "assistant" && b.type === "assistant") {
    return a.id != null && b.id != null && a.id === b.id;
  }
  if (a.type === "toolResult" && b.type === "toolResult") {
    return (
      a.results.length > 0 &&
      b.results.length > 0 &&
      a.results[0].toolCallId === b.results[0].toolCallId
    );
  }
  return false;
}

// ─── Render ─────────────────────────────────────────────────────────────────

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

  // The popover itself opens on the trigger press (nsh's `Popover` owns
  // visibility); the press also refetches the prompt list when stale, like
  // the desktop's toggle handler did.
  const openPrompts = () => {
    state.promptsOpen.set(true);
    if (state.promptsStale() || state.prompts().length === 0) {
      state.promptsStale.set(false);
      props.onRequestPrompts?.();
    }
  };

  const promptRows = (): JSX.Element[] => {
    const list = state.prompts();
    return list
      .map((prompt, at) => ({ prompt, fromEnd: list.length - 1 - at }))
      .map(({ prompt, fromEnd }) => (
        <View
          class="h-[34px] w-full flex-none flex-row items-center gap-3 rounded-lg px-3 hover:bg-accent"
          role="button"
          accessibilityLabel={`Jump to prompt ${prompt.id}`}
          onPress={() => {
            state.promptsOpen.set(false);
            props.onJumpToPrompt?.(fromEnd);
          }}
        >
          <View class="min-w-0 flex-1 overflow-hidden">
            <Text class="text-[13px] text-foreground">{prompt.prompt}</Text>
          </View>
          {(() => {
            // "just now" and dates already read fine; only "3h" needs "ago".
            const age = formatTimeAgo(prompt.createdAt);
            return prompt.createdAt > 0 ? (
              <Text class="flex-none text-[12px] text-muted-foreground">
                {age.includes(" ") ? age : `${age} ago`}
              </Text>
            ) : null;
          })()}
        </View>
      ));
  };

  const controls = (): JSX.Element | null => {
    const hasPrompts = state.messages().length > 0;
    if (!hasPrompts) return null;
    return (
      <View class="absolute bottom-3 right-4 flex-row items-center overflow-hidden rounded-[17px] border border-border bg-popover">
        <Popover
          placement="top"
          align="end"
          offset={6}
          popupClass="rounded-[10px] border border-border bg-popover p-1 shadow-md"
          popupStyle={{ width: 300, maxHeight: 7 * 34 }}
          trigger={
            <View
              class="h-8 w-8 flex-row items-center justify-center hover:bg-accent"
              role="button"
              accessibilityLabel="Jump to prompt"
              onPress={openPrompts}
            >
              <Icon name="List" size={12} color={token("muted-foreground")} />
            </View>
          }
        >
          <View class="flex-col">{promptRows()}</View>
        </Popover>
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
        <View class="relative h-full w-full flex-col">
          <View class="h-full w-full flex-col pb-11">{rows()}</View>
          {workingRow()}
          {controls()}
        </View>
      )}
    </View>
  );
}
