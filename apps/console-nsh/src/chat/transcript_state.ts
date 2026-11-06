// The transcript's mutable state and streaming API, split out of
// `transcript_view.tsx` and held as nsh signals so the rendered rows track
// them. Mirrors the desktop entity's public mutation surface; scroll and
// cache internals are not ported (see the header note on `transcript_view.tsx`).

import { signal } from "nsh";
import { changesSummary, messagesMatch, type AgentMessage, type AssistantMessage, type SessionFileChange, type SessionPrompt, type UserMessage } from "./transcript_model";
import type { AssistantContentPart } from "./message_assistant";
import type { ToolCall, ToolResult } from "./toolcalls/model";

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
    changesSummary() {
      return changesSummary(sessionChanges());
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
