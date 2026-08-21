import { create } from "zustand";
import type { ApprovalMode, ProjectInfo, SessionDetailResponse } from "@console/types";
import { sessionService } from "@console/api";
import { useProjectStore } from "./useProjectStore";
import { useProviderStore } from "./useProviderStore";
import { useSessionStatusStore } from "./useSessionStatusStore";

export interface SessionViewState {
  sessionModelId: string | null;
  sessionProvider: string | null;
  sessionCwd: string | null;
  approvalMode: ApprovalMode;
}

export const EMPTY_SESSION_VIEW: SessionViewState = {
  sessionModelId: null,
  sessionProvider: null,
  sessionCwd: null,
  approvalMode: "always-ask",
};

interface SessionState {
  sessions: Record<string, SessionViewState>;

  loadSession: (sessionId: string) => Promise<SessionDetailResponse | null>;
  getSession: (sessionId: string) => SessionViewState;
  changeModel: (sessionId: string, modelId: string) => void;
  changeProject: (sessionId: string, project: ProjectInfo) => void;
  setApprovalMode: (sessionId: string, mode: ApprovalMode) => void;
  clear: () => void;
}

type SessionHasMessagesChecker = (sessionId: string) => boolean;
let hasMessagesChecker: SessionHasMessagesChecker | null = null;

export function registerSessionHasMessagesChecker(checker: SessionHasMessagesChecker) {
  hasMessagesChecker = checker;
}

function resolveProvider(modelId: string, fallback: string | null): string | null {
  const { providers, modelsByProvider } = useProviderStore.getState();
  for (const provider of providers) {
    if ((modelsByProvider[provider.name] ?? []).some((model) => model.id === modelId)) {
      return provider.name;
    }
  }
  return fallback;
}

export const useSessionStore = create<SessionState>((set, get) => ({
  sessions: {},

  loadSession: async (sessionId) => {
    try {
      const detail = await sessionService.getSession(sessionId);
      set((state) => ({
        sessions: {
          ...state.sessions,
          [sessionId]: {
            sessionModelId: detail.header.modelId ?? null,
            sessionProvider: detail.header.provider ?? null,
            sessionCwd: detail.header.cwd ?? null,
            approvalMode: (detail.header.approvalMode as ApprovalMode) ?? "always-ask",
          },
        },
      }));
      useSessionStatusStore.getState().setStatus(sessionId, detail.header.status ?? "idle");
      return detail;
    } catch {
      return null;
    }
  },

  getSession: (sessionId) => get().sessions[sessionId] ?? EMPTY_SESSION_VIEW,

  changeModel: (sessionId, modelId) => {
    const current = get().getSession(sessionId);
    const provider = resolveProvider(modelId, current.sessionProvider);
    set((state) => ({
      sessions: {
        ...state.sessions,
        [sessionId]: { ...current, sessionModelId: modelId, sessionProvider: provider },
      },
    }));
    sessionService
      .updateSession(sessionId, {
        modelId,
        provider: provider as import("@console/types").ProviderId | undefined,
      })
      .catch(() => {});
  },

  changeProject: (sessionId, project) => {
    // Lock the working directory once a chat has messages. Each run reloads
    // header.cwd for prompt-ref expansion, project context, and all tool
    // paths — changing it mid-chat mixes old context with a new project.
    if (hasMessagesChecker && hasMessagesChecker(sessionId)) return;
    const current = get().getSession(sessionId);
    set((state) => ({
      sessions: {
        ...state.sessions,
        [sessionId]: { ...current, sessionCwd: project.path },
      },
    }));
    sessionService
      .updateSession(sessionId, { cwd: project.path })
      .then(() => useProjectStore.getState().refreshSessionHeader(sessionId))
      .catch(() => {});
  },

  setApprovalMode: (sessionId, mode) => {
    const current = get().getSession(sessionId);
    set((state) => ({
      sessions: {
        ...state.sessions,
        [sessionId]: { ...current, approvalMode: mode },
      },
    }));
    sessionService.updateSession(sessionId, { approvalMode: mode }).catch(() => {});
  },

  clear: () => set({ sessions: {} }),
}));
