import { batch, observable } from "@legendapp/state";
import type { SessionHeader, SessionStatus } from "@console/types";

/**
 * Per-session run status, keyed by session id.
 *
 * First store migrated from zustand to Legend State (see
 * docs/legend-state-and-list-migration.md, Phase B1). Reads in components use
 * `useValue(sessionStatuses$)`; writes use the exported actions below.
 */
export const sessionStatuses$ = observable<Record<string, SessionStatus>>({});

/** Seed statuses for sessions that don't have one yet (does not overwrite). */
export function setStatuses(sessions: SessionHeader[]): void {
  batch(() => {
    const current = sessionStatuses$.peek();
    for (const session of sessions) {
      if (!(session.id in current)) {
        sessionStatuses$[session.id].set(session.status ?? "idle");
      }
    }
  });
}

export function setStatus(sessionId: string, status: SessionStatus): void {
  sessionStatuses$[sessionId].set(status);
}

export function clearStatus(sessionId: string): void {
  sessionStatuses$[sessionId].delete();
}
