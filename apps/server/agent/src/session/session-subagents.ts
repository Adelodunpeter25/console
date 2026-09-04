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

function computeActivitySummary(args: Record<string, unknown> | undefined): string | undefined {
  if (!args || typeof args !== "object") return undefined;
  const val =
    args.command ??
    args.CommandLine ??
    args.path ??
    args.AbsolutePath ??
    args.SearchDirectory ??
    args.TargetFile ??
    args.pattern ??
    args.Pattern ??
    args.Query ??
    args.query ??
    args.url ??
    args.Url ??
    args.question ??
    args.directory ??
    args.SearchPath ??
    args.Prompt ??
    args.prompt ??
    args.filePath ??
    args.targetFile ??
    args.absolutePath;

  if (val != null) {
    const s = String(val).trim().replace(/\s+/g, " ");
    return s.length > 70 ? s.slice(0, 67) + "…" : s;
  }

  const firstKey = Object.keys(args)[0];
  if (firstKey) {
    const fv = String(args[firstKey]).trim().replace(/\s+/g, " ");
    const preview = fv.length > 50 ? fv.slice(0, 47) + "…" : fv;
    return `${firstKey}: ${preview}`;
  }
  return undefined;
}

function compactArgs(args: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!args || typeof args !== "object") return undefined;
  const compacted: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(args)) {
    if (typeof v === "string") {
      compacted[k] = v.length > 200 ? v.slice(0, 197) + "…" : v;
    } else {
      compacted[k] = v;
    }
  }
  return compacted;
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

  const mergedArgs = event.args ?? existing?.args;
  const summary = computeActivitySummary(mergedArgs) ?? existing?.summary;

  const activityItem: SubagentActivityItem = {
    turnIndex: event.turnIndex,
    toolCallId: event.toolCallId,
    toolName: event.toolName || existing?.toolName || "",
    summary,
    args: compactArgs(mergedArgs),
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

export const SUBAGENT_COMPLETED_RETENTION_MS = 10_000;

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

  // Schedule deletion after 10 seconds
  setTimeout(() => {
    try {
      sessionDb
        .prepare("DELETE FROM session_subagents WHERE id = ? AND status != 'running'")
        .run(event.subagentId);
    } catch {
      // Session database may have closed or session deleted
    }
  }, SUBAGENT_COMPLETED_RETENTION_MS).unref?.();
}

export function getSessionSubagents(
  state: StorageState,
  sessionId: string,
): SubagentInfo[] {
  const projectId = getProjectIdBySessionId(state.globalDb, sessionId);
  if (projectId === undefined) return [];

  const sessionDb = getSessionDb(state, sessionId, projectId);
  const cutoff = Date.now() - SUBAGENT_COMPLETED_RETENTION_MS;

  // Clean up any stale completed/aborted/error subagents older than cutoff
  try {
    sessionDb
      .prepare("DELETE FROM session_subagents WHERE status != 'running' AND updated_at < ?")
      .run(cutoff);
  } catch {
    // Ignore cleanup error
  }

  const rows = sessionDb
    .prepare(
      "SELECT * FROM session_subagents WHERE status = 'running' OR updated_at >= ? ORDER BY created_at ASC",
    )
    .all(cutoff) as SubagentRow[];

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
