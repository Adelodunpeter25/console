// Sidebar grouping, ported from console-ui's `utils/session_groups.rs`:
// calendar buckets in display order, most recent first, empty buckets
// omitted. Pinned sessions surface above the buckets (the GPUI app renders
// them via the pin path in `sidebar/mod.rs`).

import type { SessionSummary } from "console-nsh/native";

export type SessionGroup = { label: string; sessions: SessionSummary[] };

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
    if (session.status === "pinned") {
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
/// `group_indices_by_project`: one bucket per project id, "none" last, each
/// sorted most recent first. `projectName` resolves a bucket's label; the
/// GPUI app falls back to the working-directory name, which the summary
/// does not carry yet, so unknown projects group as "Other".
export function groupSessionsByProject(
  sessions: SessionSummary[],
  projectName: (projectId: string | null) => string | null,
): SessionGroup[] {
  const byProject = new Map<string, SessionSummary[]>();
  for (const session of sessions) {
    const key = session.projectId ?? "none";
    const list = byProject.get(key) ?? [];
    list.push(session);
    byProject.set(key, list);
  }

  const groups: SessionGroup[] = [];
  const none = byProject.get("none");
  if (none !== undefined) byProject.delete("none");
  for (const [projectId, items] of byProject) {
    items.sort((a, b) => b.updatedAt - a.updatedAt);
    groups.push({
      label: projectName(projectId === "none" ? null : projectId) ?? "Other",
      sessions: items,
    });
  }
  if (none !== undefined) {
    none.sort((a, b) => b.updatedAt - a.updatedAt);
    groups.push({ label: "None", sessions: none });
  }
  return groups;
}
