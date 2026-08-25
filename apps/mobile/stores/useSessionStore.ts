import { batch, observable } from "@legendapp/state";
import type { ApprovalMode, ProjectInfo, SessionDetailResponse } from "@console/types";
import { sessionService } from "@console/api";
import { provider$ } from "./useProviderStore";
import { setStatus } from "./useSessionStatusStore";
import { useProjectStore } from "./useProjectStore";

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

/**
 * Per-session header view state (model/provider/cwd/approval mode) as Legend
 * State observables, keyed by session id. See
 * docs/legacy-state-and-list-migration.md.
 */
export const sessionsView$ = observable<Record<string, SessionViewState>>({});

type SessionHasMessagesChecker = (sessionId: string) => boolean;
let hasMessagesChecker: SessionHasMessagesChecker | null = null;

export function registerSessionHasMessagesChecker(checker: SessionHasMessagesChecker) {
  hasMessagesChecker = checker;
}

function resolveProvider(modelId: string, fallback: string | null): string | null {
  const { providers, modelsByProvider } = provider$.peek();
  for (const provider of providers) {
    if ((modelsByProvider[provider.name] ?? []).some((model) => model.id === modelId)) {
      return provider.name;
    }
  }
  return fallback;
}

/** Read a session's view state without subscribing (falls back to defaults). */
export function getSession(sessionId: string): SessionViewState {
  return sessionsView$[sessionId].peek() ?? EMPTY_SESSION_VIEW;
}

export async function loadSession(sessionId: string): Promise<SessionDetailResponse | null> {
  try {
    const detail = await sessionService.getSession(sessionId);
    sessionsView$[sessionId].set({
      sessionModelId: detail.header.modelId ?? null,
      sessionProvider: detail.header.provider ?? null,
      sessionCwd: detail.header.cwd ?? null,
      approvalMode: (detail.header.approvalMode as ApprovalMode) ?? "always-ask",
    });
    setStatus(sessionId, detail.header.status ?? "idle");
    return detail;
  } catch {
    return null;
  }
}

export function changeModel(sessionId: string, modelId: string): void {
  const current = getSession(sessionId);
  const provider = resolveProvider(modelId, current.sessionProvider);
  sessionsView$[sessionId].set({
    ...current,
    sessionModelId: modelId,
    sessionProvider: provider,
  });
  sessionService
    .updateSession(sessionId, {
      modelId,
      provider: provider as import("@console/types").ProviderId | undefined,
    })
    .catch(() => {});
}

export function changeProject(sessionId: string, project: ProjectInfo): void {
  // Lock the working directory once a chat has messages. Each run reloads
  // header.cwd for prompt-ref expansion, project context, and all tool
  // paths — changing it mid-chat mixes old context with a new project.
  if (hasMessagesChecker && hasMessagesChecker(sessionId)) return;
  const current = getSession(sessionId);
  sessionsView$[sessionId].set({ ...current, sessionCwd: project.path });
  sessionService
    // projectId deliberately omitted: the server PATCH contract has no
    // project field and would silently ignore it (see session.service.ts).
    .updateSession(sessionId, { cwd: project.path })
    .then(() => useProjectStore.getState().refreshSessionHeader(sessionId))
    .catch(() => {});
}

export function setApprovalMode(sessionId: string, mode: ApprovalMode): void {
  const current = getSession(sessionId);
  sessionsView$[sessionId].set({ ...current, approvalMode: mode });
  sessionService.updateSession(sessionId, { approvalMode: mode }).catch(() => {});
}

export function clearSessions(): void {
  sessionsView$.set({});
}
