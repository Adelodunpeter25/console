import { create } from "zustand";
import type { SessionHeader, SessionStatus } from "@console/types";

interface SessionStatusState {
  statuses: Record<string, SessionStatus>;
  setStatuses: (sessions: SessionHeader[]) => void;
  setStatus: (sessionId: string, status: SessionStatus) => void;
  clearStatus: (sessionId: string) => void;
}

export const useSessionStatusStore = create<SessionStatusState>((set) => ({
  statuses: {},

  setStatuses: (sessions) =>
    set((state) => {
      const statuses = { ...state.statuses };
      for (const session of sessions) {
        if (!(session.id in statuses)) statuses[session.id] = session.status ?? "idle";
      }
      return { statuses };
    }),

  setStatus: (sessionId, status) =>
    set((state) => ({ statuses: { ...state.statuses, [sessionId]: status } })),

  clearStatus: (sessionId) =>
    set((state) => {
      const statuses = { ...state.statuses };
      delete statuses[sessionId];
      return { statuses };
    }),
}));
