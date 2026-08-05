import { create } from "zustand";
import type { ApprovalMode, ProjectInfo, SessionDetailResponse } from "@console/types";
import { tauriApi } from "../lib/tauri-api";
import { useAppStore } from "./useAppStore";
import { useProjectStore } from "./useProjectStore";
import { useProviderStore } from "./useProviderStore";
import { useSessionStatusStore } from "./useSessionStatusStore";

interface SessionState {
  loading: boolean;
  sessionModelId: string | null;
  sessionProvider: string | null;
  sessionCwd: string | null;
  approvalMode: ApprovalMode;

  loadSession: (sessionId: string) => Promise<SessionDetailResponse | null>;
  changeModel: (sessionId: string, modelId: string) => void;
  changeProject: (sessionId: string, project: ProjectInfo) => void;
  setApprovalMode: (mode: ApprovalMode) => void;
  clear: () => void;
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

const initialSessionState = {
  loading: false,
  sessionModelId: null,
  sessionProvider: null,
  sessionCwd: null,
  approvalMode: "always-ask" as ApprovalMode,
};

export const useSessionStore = create<SessionState>((set, get) => ({
  ...initialSessionState,

  loadSession: async (sessionId) => {
    set({ loading: true });
    try {
      const detail = await tauriApi.getSession(sessionId);
      if (useAppStore.getState().selectedSessionId !== sessionId) return null;

      set({
        loading: false,
        sessionModelId: detail.header.modelId ?? null,
        sessionProvider: detail.header.provider ?? null,
        sessionCwd: detail.header.cwd ?? null,
        approvalMode: (detail.header.approvalMode as ApprovalMode) ?? "always-ask",
      });
      useSessionStatusStore.getState().setStatus(sessionId, detail.header.status ?? "idle");
      return detail;
    } catch {
      if (useAppStore.getState().selectedSessionId === sessionId) {
        set({ ...initialSessionState });
      }
      return null;
    } finally {
      if (useAppStore.getState().selectedSessionId === sessionId) {
        set({ loading: false });
      }
    }
  },

  changeModel: (sessionId, modelId) => {
    const provider = resolveProvider(modelId, get().sessionProvider);
    set({ sessionModelId: modelId, sessionProvider: provider });
    tauriApi
      .updateSession(sessionId, {
        modelId,
        provider: provider as "gemini" | "antigravity" | undefined,
      })
      .catch(() => {});
  },

  changeProject: (sessionId, project) => {
    set({ sessionCwd: project.path });
    useAppStore.getState().setSelectedProjectId(project.id);
    tauriApi
      .updateSession(sessionId, { cwd: project.path })
      .then(() => useProjectStore.getState().refreshSessionHeader(sessionId))
      .catch(() => {});
  },

  setApprovalMode: (mode) => {
    set({ approvalMode: mode });
    const sessionId = useAppStore.getState().selectedSessionId;
    if (sessionId) {
      tauriApi.updateSession(sessionId, { approvalMode: mode }).catch(() => {});
    }
  },

  clear: () => set({ ...initialSessionState }),
}));
