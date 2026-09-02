import type { AgentMessage, AgentSessionEvent, ToolResult } from "@console/types";
import type {
  ActivityEvent,
  ChatSessionState,
  ChatSnapshot,
  RunActivityState,
} from "@/types/chat";

/** Unique per-message id (crypto.randomUUID with a fallback). */
export function newMessageId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/**
 * Ensure every message carries a unique id.
 *
 * Legend State keys observable arrays by `id` and warns when elements collide;
 * assistant messages get ids from the server but user/toolResult messages are
 * created without one. Returns the same array reference when nothing changed
 * so callers don't churn object identities unnecessarily.
 */
export function ensureMessageIds(messages: AgentMessage[]): AgentMessage[] {
  let changed = false;
  const out = messages.map((m) => {
    if (m.id) return m;
    changed = true;
    return { ...m, id: newMessageId() };
  });
  return changed ? out : messages;
}

/** Update the latest run in the session's runs array. */
function updateLatestRun(
  session: ChatSessionState,
  update: (run: RunActivityState) => RunActivityState,
): ChatSessionState {
  if (session.runs.length === 0) return session;
  const runs = [...session.runs];
  runs[runs.length - 1] = update(runs[runs.length - 1]!);
  return { ...session, runs };
}

/** Update a specific tool call event's result by toolCallId. */
function setToolCallResult(events: ActivityEvent[], result: ToolResult): ActivityEvent[] {
  return events.map((event) =>
    event.type === "toolCall" && event.call.id === result.toolCallId
      ? { ...event, result }
      : event,
  );
}

/** Finalize any tool call events that never received a result. */
function finalizePendingToolCalls(events: ActivityEvent[]): ActivityEvent[] {
  return events.map((event) =>
    event.type === "toolCall" && !event.result
      ? {
          ...event,
          result: {
            toolCallId: event.call.id,
            toolName: event.call.name,
            content: "Run ended before this tool call completed.",
            isError: true,
          },
        }
      : event,
  );
}

export function applyChatEvent(
  session: ChatSessionState,
  event: AgentSessionEvent,
): ChatSessionState {
  switch (event.type) {
    case "modelStreamPart": {
      const text = event.part?.text;
      const thinking = event.part?.thinking;
      if (!text && !thinking) return session;
      return {
        ...session,
        streamingText: text ? session.streamingText + text : session.streamingText,
        streamingThinking: thinking
          ? session.streamingThinking + thinking
          : session.streamingThinking,
      };
    }
    case "modelStreamEnd": {
      const turn = event.turn;
      if (!turn) {
        // Fallback: construct from streaming buffers if no turn was provided.
        if (!session.streamingText && !session.streamingThinking) return session;
        return {
          ...session,
          messages: [
            ...session.messages,
            {
              role: "assistant",
              id: newMessageId(),
              createdAt: Date.now(),
              content: [
                ...(session.streamingThinking
                  ? [{ type: "thinking" as const, text: session.streamingThinking }]
                  : []),
                ...(session.streamingText
                  ? [{ type: "text" as const, text: session.streamingText }]
                  : []),
              ],
            },
          ],
          streamingText: "",
          streamingThinking: "",
        };
      }

      const turnWithMeta = turn as typeof turn & { createdAt?: number; id?: string };
      const normalizedTurn = {
        ...turnWithMeta,
        // Turns may arrive without an id (provider-dependent) — assign one so
        // every stored message is uniquely keyed.
        id: turnWithMeta.id ?? newMessageId(),
        createdAt: turnWithMeta.createdAt ?? Date.now(),
      };

      // Always append the turn to messages for persistence.
      const baseResult: ChatSessionState = {
        ...session,
        messages: [...session.messages, normalizedTurn],
        streamingText: "",
        streamingThinking: "",
      };

      // If the turn has tool calls, extract text and tool call parts as
      // timeline events in the latest run. The text becomes "progress text"
      // inside the run activity, not a standalone message bubble.
      const toolCallParts = turn.content.filter(
        (c): c is Extract<typeof c, { type: "toolCall" }> => c.type === "toolCall",
      );

      if (toolCallParts.length === 0) {
        // No tool calls — this is the final response, not part of the timeline.
        return baseResult;
      }

      // Build timeline events in content order: thinking/text parts become
      // thinking/text events, tool call parts become toolCall events (with no
      // result yet). This preserves the chronological think → tool → think →
      // tool order from the agent loop.
      const newEvents: ActivityEvent[] = [];
      for (const part of turn.content) {
        if (part.type === "thinking" && part.text.trim()) {
          newEvents.push({
            type: "thinking",
            id: `thinking-${part.text.slice(0, 16)}-${Date.now()}`,
            text: part.text,
          });
        } else if (part.type === "text" && part.text.trim()) {
          newEvents.push({
            type: "text",
            id: `text-${part.text.slice(0, 16)}-${Date.now()}`,
            text: part.text,
          });
        } else if (part.type === "toolCall") {
          newEvents.push({ type: "toolCall", id: part.call.id, call: part.call });
        }
      }

      // Set active tool calls for toolExecutionEnd error finalization.
      baseResult.activeToolCalls = toolCallParts.map((c) => c.call);

      return updateLatestRun(baseResult, (run) => ({
        ...run,
        events: [...run.events, ...newEvents],
      }));
    }
    case "toolExecutionResult":
      return updateLatestRun(session, (run) => ({
        ...run,
        events: setToolCallResult(run.events, event.result),
      }));
    case "toolExecutionStart": {
      // Tool calls were already added as events in modelStreamEnd.
      // Just track active calls for toolExecutionEnd finalization.
      return { ...session, activeToolCalls: event.calls };
    }
    case "toolExecutionEnd": {
      // Build the complete results list for this batch.
      const eventResults = [...event.results];
      // Add error results for any active calls that didn't get a result.
      for (const call of session.activeToolCalls) {
        if (!eventResults.some((r) => r.toolCallId === call.id)) {
          eventResults.push({
            toolCallId: call.id,
            toolName: call.name,
            content: "Tool execution ended without a result.",
            isError: true,
          });
        }
      }

      // Update each tool call event with its result.
      let updated = updateLatestRun(session, (run) => ({
        ...run,
        events: eventResults.reduce(setToolCallResult, run.events),
      }));

      // Append the toolResult message to messages for persistence transport.
      // It renders as null in the UI — results are shown via RunActivity.
      updated = {
        ...updated,
        messages: [
          ...updated.messages,
          { role: "toolResult", id: newMessageId(), results: eventResults },
        ],
        activeToolCalls: [],
      };

      return updated;
    }
    case "askQuestion":
      return {
        ...session,
        pendingQuestions: [...session.pendingQuestions, { request: event.request }],
      };
    case "permissionRequest":
      return {
        ...session,
        pendingPermissions: [...session.pendingPermissions, { request: event.request }],
      };
    case "todoUpdate":
      return { ...session, todoItems: event.items };
    case "sessionEnd":
      // Finalize the latest run: mark as completed if still working, and
      // add error results for any tool calls that never received a result.
      return updateLatestRun(session, (run) => {
        if (run.status !== "working") return run;
        return {
          ...run,
          status: "completed",
          events: finalizePendingToolCalls(run.events),
          elapsedMs: run.startedAt ? Date.now() - run.startedAt : run.elapsedMs,
        };
      });
    case "subagentStart": {
      const subagents = session.subagents ? [...session.subagents] : [];
      const existingIdx = subagents.findIndex((s) => s.subagentId === event.subagentId);
      if (existingIdx >= 0) {
        subagents[existingIdx] = {
          ...subagents[existingIdx]!,
          name: event.name,
          role: event.role,
          prompt: event.prompt,
          maxTurns: event.maxTurns,
          status: "running",
          updatedAt: Date.now(),
        };
      } else {
        subagents.push({
          subagentId: event.subagentId,
          parentToolCallId: event.parentToolCallId,
          name: event.name,
          role: event.role,
          prompt: event.prompt,
          maxTurns: event.maxTurns,
          currentTurn: 1,
          status: "running",
          activities: [],
          createdAt: Date.now(),
          updatedAt: Date.now(),
        });
      }
      return {
        ...session,
        subagents,
      };
    }
    case "subagentActivity": {
      const subagents = session.subagents ? [...session.subagents] : [];
      const idx = subagents.findIndex((s) => s.subagentId === event.subagentId);
      if (idx >= 0) {
        const target = { ...subagents[idx]! };
        target.currentTurn = Math.max(target.currentTurn, event.turnIndex);
        const activities = [...target.activities];
        const actIdx = activities.findIndex((a) => a.toolCallId === event.toolCallId);
        const existing = actIdx >= 0 ? activities[actIdx] : undefined;
        const item = {
          turnIndex: event.turnIndex,
          toolCallId: event.toolCallId,
          toolName: event.toolName || existing?.toolName || "",
          args: event.args ?? existing?.args,
          status: event.status,
          error: event.error ?? existing?.error,
        };
        if (actIdx >= 0) {
          activities[actIdx] = item;
        } else {
          activities.push(item);
        }
        target.activities = activities;
        target.updatedAt = Date.now();
        subagents[idx] = target;
      }
      return {
        ...session,
        subagents,
      };
    }
    case "subagentEnd": {
      const subagents = session.subagents ? [...session.subagents] : [];
      const idx = subagents.findIndex((s) => s.subagentId === event.subagentId);
      if (idx >= 0) {
        const target = { ...subagents[idx]! };
        target.status = event.status;
        if (event.summary !== undefined) target.summary = event.summary;
        if (event.error !== undefined) target.error = event.error;
        if (event.totalTurns > 0) target.currentTurn = event.totalTurns;
        target.updatedAt = Date.now();
        subagents[idx] = target;
      }
      return {
        ...session,
        subagents,
      };
    }
    case "streamReset":
      // Re-attach replay is about to arrive — clear streaming buffers so the
      // replayed coalesced snapshots don't duplicate already-seen text.
      return {
        ...session,
        streamingText: "",
        streamingThinking: "",
      };
    case "error":
      if (event.error?.message.toLowerCase().includes("aborted")) {
        return updateLatestRun(session, (run) => ({
          ...run,
          status: run.status === "working" ? "aborted" : run.status,
          events: finalizePendingToolCalls(run.events),
        }));
      }
      return {
        ...updateLatestRun(session, (run) => ({
          ...run,
          status: run.status === "working" ? "failed" : run.status,
          events: finalizePendingToolCalls(run.events),
        })),
        messages: [
          ...session.messages,
          {
            role: "assistant",
            id: newMessageId(),
            content: [
              { type: "text", text: `Error: ${event.error?.message ?? "Unknown agent error"}` },
            ],
          },
        ],
        streamingText: "",
        streamingThinking: "",
      };
    default:
      return session;
  }
}

/**
 * Derive the UI snapshot for a session.
 *
 * The store holds the full per-session runtime state (queues, runs timeline,
 * todo items, attachments). The snapshot flattens it to what screens render:
 * a single pending permission/question (the most recent), live tool results
 * from the latest run, and the streaming buffers.
 *
 * Snapshots are cached per session-state reference so selectors handed to
 * `useSyncExternalStore` return a stable object identity for unchanged state.
 * Without this, React 19 reports "The result of getSnapshot should be cached"
 * and can loop with "Maximum update depth exceeded".
 */
const snapshotCache = new WeakMap<ChatSessionState, ChatSnapshot>();

export function toChatSnapshot(session: ChatSessionState): ChatSnapshot {
  const cached = snapshotCache.get(session);
  if (cached) return cached;

  const snapshot: ChatSnapshot = {
    messages: session.messages,
    streamingText: session.streamingText,
    streamingThinking: session.streamingThinking,
    activeToolCalls: session.activeToolCalls,
    liveToolResults: (session.runs[session.runs.length - 1]?.events ?? [])
      .filter(
        (event): event is Extract<ActivityEvent, { type: "toolCall" }> =>
          event.type === "toolCall" && Boolean(event.result),
      )
      .map((event) => event.result!),
    pendingPermission: session.pendingPermissions[session.pendingPermissions.length - 1] ?? null,
    pendingQuestion: session.pendingQuestions[session.pendingQuestions.length - 1] ?? null,
    pendingPermissions: session.pendingPermissions,
    pendingQuestions: session.pendingQuestions,
    todoItems: session.todoItems ?? [],
    subagents: session.subagents ?? [],
    running: session.running,
    runs: session.runs,
  };

  snapshotCache.set(session, snapshot);
  return snapshot;
}
