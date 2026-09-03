import type {
  SubagentInfo,
  SubagentActivityItem,
  SubagentStartEvent,
  SubagentActivityEvent,
  SubagentEndEvent,
} from "@console/types";
import { getProjectIdBySessionId, getSessionDb } from "./session-helpers.js";
import type { StorageState } from "./utils.js";

interface SubagentRow {
  id: string;
  parent_tool_call_id: string;
  name: string;
  role: string;
  prompt: string;
  max_turns: number;
  current_turn: number;
  status: string;
  summary: string | null;
  error: string | null;
  activities: string;
  created_at: number;
  updated_at: number;
}

export function upsertSubagentStart(
  state: StorageState,
  sessionId: string,
  event: SubagentStartEvent,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const now = Date.now();

  const stmt = sessionDb.prepare(`
    INSERT INTO session_subagents (
      id, parent_tool_call_id, name, role, prompt, max_turns, current_turn, status, summary, error, activities, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
      name = excluded.name,
      role = excluded.role,
      prompt = excluded.prompt,
      max_turns = excluded.max_turns,
      status = excluded.status,
      updated_at = excluded.updated_at
  `);

  stmt.run(
    event.subagentId,
    event.parentToolCallId,
    event.name,
    event.role,
    event.prompt,
    event.maxTurns,
    1,
    "running",
    null,
    null,
    "[]",
    now,
    now,
  );
}

export function appendSubagentActivity(
  state: StorageState,
  sessionId: string,
  event: SubagentActivityEvent,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const now = Date.now();

  const row = sessionDb
    .prepare("SELECT current_turn, activities FROM session_subagents WHERE id = ?")
    .get(event.subagentId) as { current_turn: number; activities: string } | undefined;

  let activities: SubagentActivityItem[] = [];
  let currentTurn = event.turnIndex;

  if (row) {
    try {
      activities = JSON.parse(row.activities);
    } catch {
      activities = [];
    }
    currentTurn = Math.max(row.current_turn, event.turnIndex);
  }

  const existingIdx = activities.findIndex((a) => a.toolCallId === event.toolCallId);
  const existing = existingIdx >= 0 ? activities[existingIdx] : undefined;

  const activityItem: SubagentActivityItem = {
    turnIndex: event.turnIndex,
    toolCallId: event.toolCallId,
    toolName: event.toolName || existing?.toolName || "",
    args: event.args ?? existing?.args,
    status: event.status,
    error: event.error ?? existing?.error,
  };

  if (existingIdx >= 0) {
    activities[existingIdx] = activityItem;
  } else {
    activities.push(activityItem);
  }

  const stmt = sessionDb.prepare(`
    UPDATE session_subagents
    SET current_turn = ?, activities = ?, updated_at = ?
    WHERE id = ?
  `);

  stmt.run(currentTurn, JSON.stringify(activities), now, event.subagentId);
}

export function completeSubagent(
  state: StorageState,
  sessionId: string,
  event: SubagentEndEvent,
): void {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return;

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const now = Date.now();

  const stmt = sessionDb.prepare(`
    UPDATE session_subagents
    SET status = ?, summary = COALESCE(?, summary), error = COALESCE(?, error), current_turn = CASE WHEN ? > 0 THEN ? ELSE current_turn END, updated_at = ?
    WHERE id = ?
  `);

  stmt.run(
    event.status,
    event.summary ?? null,
    event.error ?? null,
    event.totalTurns,
    event.totalTurns,
    now,
    event.subagentId,
  );
}

export function getSessionSubagents(
  state: StorageState,
  sessionId: string,
): SubagentInfo[] {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return [];

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const rows = sessionDb
    .prepare("SELECT * FROM session_subagents ORDER BY created_at ASC")
    .all() as SubagentRow[];

  return rows.map((r) => {
    let activities: SubagentActivityItem[] = [];
    try {
      activities = JSON.parse(r.activities);
    } catch {
      activities = [];
    }

    return {
      subagentId: r.id,
      parentToolCallId: r.parent_tool_call_id,
      name: r.name,
      role: r.role,
      prompt: r.prompt,
      maxTurns: r.max_turns,
      currentTurn: r.current_turn,
      status: r.status as SubagentInfo["status"],
      summary: r.summary ?? undefined,
      error: r.error ?? undefined,
      activities,
      createdAt: r.created_at,
      updatedAt: r.updated_at,
    };
  });
}
