// Grouping for the sidebar's chat list, the shape of console-ui's
// `utils/session_groups.rs`: pinned first, then recency buckets.

import type { SessionSummary } from "console-nsh/native";

export type SessionGroup = { label: string; sessions: SessionSummary[] };

const DAY = 24 * 60 * 60 * 1000;

export function groupSessions(
  sessions: SessionSummary[],
  now: number = Date.now(),
): SessionGroup[] {
  const sorted = [...sessions].sort((a, b) => b.updatedAt - a.updatedAt);
  const groups: SessionGroup[] = [
    { label: "Pinned", sessions: [] },
    { label: "Today", sessions: [] },
    { label: "Yesterday", sessions: [] },
    { label: "This week", sessions: [] },
    { label: "Older", sessions: [] },
  ];
  for (const session of sorted) {
    const age = now - session.updatedAt;
    if (session.status === "pinned") {
      groups[0].sessions.push(session);
    } else if (age < DAY) {
      groups[1].sessions.push(session);
    } else if (age < 2 * DAY) {
      groups[2].sessions.push(session);
    } else if (age < 7 * DAY) {
      groups[3].sessions.push(session);
    } else {
      groups[4].sessions.push(session);
    }
  }
  return groups.filter((g) => g.sessions.length > 0);
}
