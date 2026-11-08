// Sidebar grouping, ported from console-ui's `utils/session_groups.rs`:
// calendar buckets in display order, most recent first, empty buckets
// omitted. Pinned sessions surface above the buckets (the GPUI app renders
// them via the pin path in `sidebar/mod.rs`).

import type { SessionSummary } from "console-nsh/native";

export type SessionGroup = { label: string; sessions: SessionSummary[] };

/// Project names show with a capital first letter without changing the
/// persisted name (console-ui's `sentence_case_project_name`).
export function sentenceCaseProjectName(name: string): string {
  return name === "" ? name : name[0].toUpperCase() + name.slice(1);
}

/// Settled chats leave every active section and gather in their own list at
/// the bottom, most recent first (`partition_settled`).
export function partitionSettled(sessions: SessionSummary[]): {
  active: SessionSummary[];
  settled: SessionSummary[];
} {
  const active = sessions.filter((s) => !s.settled);
  const settled = sessions
    .filter((s) => s.settled)
    .sort((a, b) => b.updatedAt - a.updatedAt);
  return { active, settled };
}

const LABELS = [
  "Today",
  "Yesterday",
  "This week",
  "This month",
  "Older",
] as const;

function bucket(updatedAtMs: number, today: Date): number {
  const sessionDate = new Date(updatedAtMs);
  const startOfDay = (d: Date) =>
    new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const sessionDay = startOfDay(sessionDate);
  if (sessionDay >= startOfDay(today)) return 0;

  const yesterday = new Date(today);
  yesterday.setDate(yesterday.getDate() - 1);
  if (sessionDay >= startOfDay(yesterday)) return 1;

  const weekStart = new Date(today);
  // Monday-start week, as `weekday().num_days_from_monday()` does.
  weekStart.setDate(weekStart.getDate() - ((today.getDay() + 6) % 7));
  if (sessionDay >= startOfDay(weekStart)) return 2;

  if (
    sessionDate.getFullYear() === today.getFullYear() &&
    sessionDate.getMonth() === today.getMonth()
  ) {
    return 3;
  }
  return 4;
}

export function groupSessions(
  sessions: SessionSummary[],
  now: Date = new Date(),
): SessionGroup[] {
  const pinned: SessionSummary[] = [];
  const buckets: SessionSummary[][] = LABELS.map(() => []);
  for (const session of sessions) {
    if (session.pinned) {
      pinned.push(session);
      continue;
    }
    buckets[bucket(session.updatedAt, now)].push(session);
  }

  const groups: SessionGroup[] = [];
  if (pinned.length > 0) {
    pinned.sort((a, b) => b.updatedAt - a.updatedAt);
    groups.push({ label: "Pinned", sessions: pinned });
  }
  buckets.forEach((items, index) => {
    if (items.length === 0) return;
    items.sort((a, b) => b.updatedAt - a.updatedAt);
    groups.push({ label: LABELS[index], sessions: items });
  });
  return groups;
}

/// Group chats by project, in the shape of console-ui's
/// `group_indices_by_project`: one bucket per project id, sections ordered by
/// their most recent activity (newest first), and chats without a project —
/// or whose project is unknown, per `ProjectSectionKey` — always last, under
/// "No Project" (`sidebar/mod.rs`'s `NO_PROJECT_KEY` / `"No Project"`).
/// `projectName` resolves a bucket's label from the project list; unknown
/// ids fold into the no-project section, like the GPUI app.
export function groupSessionsByProject(
  sessions: SessionSummary[],
  projectName: (projectId: string) => string | null,
): SessionGroup[] {
  const noProjectKey = "none";
  const byProject = new Map<string, SessionSummary[]>();
  for (const session of sessions) {
    // Unknown project ids fall into the no-project section.
    const raw = session.projectId;
    const key =
      raw !== null && raw !== undefined && projectName(raw) !== null
        ? raw
        : noProjectKey;
    const list = byProject.get(key) ?? [];
    list.push(session);
    byProject.set(key, list);
  }

  const groups: { group: SessionGroup; latest: number; isNoProject: boolean }[] = [];
  for (const [projectId, items] of byProject) {
    items.sort((a, b) => b.updatedAt - a.updatedAt);
    groups.push({
      group: {
        label:
          projectId === noProjectKey
            ? "No Project"
            : sentenceCaseProjectName(projectName(projectId) ?? "No Project"),
        sessions: items,
      },
      latest: items[0]?.updatedAt ?? 0,
      isNoProject: projectId === noProjectKey,
    });
  }
  groups.sort((a, b) => {
    if (a.isNoProject !== b.isNoProject) return a.isNoProject ? 1 : -1;
    return b.latest - a.latest;
  });
  return groups.map((g) => g.group);
}
