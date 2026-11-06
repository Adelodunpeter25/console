// The transcript's data model, split out of `transcript_view.tsx`: the
// message types (console-core's `AgentMessage`), run-activity grouping
// (`tool_activity_for_user`), and per-turn change grouping
// (`group_turn_changes`). The desktop's state lives on a GPUI entity; these
// are the pure derivations over the same shapes.

import type { ActivityEvent, ToolCall, ToolResult } from "./toolcalls/model";
import { elapsedSeconds as elapsedSecondsBetween } from "./toolcalls/model";
import type { AssistantContentPart } from "./message_assistant";
import type { BrowserElementAnnotation, ImageAttachment } from "./message_user";
import {
  changesSummaryFrom,
  changesSummaryIsEmpty,
  turnChangeFrom,
  type TurnChanges,
  type ChangesSummary,
} from "./change_chips";

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

/// The message's server-assigned id, when its kind carries one.
export function messageId(message: AgentMessage): string | null {
  return message.type === "toolResult" ? null : message.id ?? null;
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

/// Whole-session change totals for the pinned summary pill above the
/// composer. `undefined` when the session has not touched any file.
export function changesSummary(
  changes: SessionFileChange[],
): ChangesSummary | undefined {
  const summary = changesSummaryFrom(changes);
  return changesSummaryIsEmpty(summary) ? undefined : summary;
}

/// Whether two messages are the same one seen twice (a page overlap).
export function messagesMatch(a: AgentMessage, b: AgentMessage): boolean {
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
