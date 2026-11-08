// The one seam between the app and the native bridge. Only this file may
// import `console-nsh/native` (value import): every bridge call goes
// through here so there is a single place that maps wire results to typed
// values, classifies failures (unreachable / auth / server), and could host
// cross-cutting concerns such as a 401 → pairing-gate interceptor.

import { Console } from "console-nsh/native";

export type BackendErrorKind = "unreachable" | "auth" | "server";

export class BackendError extends Error {
  readonly kind: BackendErrorKind;

  constructor(kind: BackendErrorKind, message: string) {
    super(message);
    this.kind = kind;
  }
}

function classify(err: unknown): BackendError {
  const message = err instanceof Error ? err.message : String(err);
  if (message.includes("Authentication required")) {
    return new BackendError("auth", message);
  }
  if (message.includes("unreachable")) {
    return new BackendError("unreachable", message);
  }
  return new BackendError("server", message);
}

async function parsed<T>(raw: () => Promise<T>): Promise<T> {
  try {
    return await raw();
  } catch (err) {
    throw classify(err);
  }
}

const text = (raw: Promise<string>) => parsed(() => raw);
const json = <T>(raw: Promise<string>) => text(raw).then((s) => JSON.parse(s) as T);

export const backend = {
  // Health
  health: (baseUrl: string) => text(Console.health(baseUrl)),

  // Projects and sessions (the bridge already returns these parsed).
  projects: (baseUrl: string) => parsed(() => Console.projects(baseUrl)),
  sessions: (baseUrl: string, projectId: string) =>
    parsed(() => Console.sessions(baseUrl, projectId)),
  deletedSessions: (baseUrl: string, projectId: string | null) =>
    parsed(() => Console.deletedSessions(baseUrl, projectId)),

  createSession: (baseUrl: string, projectId: string, title: string | null) =>
    json<{ id: string }>(Console.createSession(baseUrl, projectId, title)),
  chatMessages: (baseUrl: string, sessionId: string) =>
    text(Console.chatMessages(baseUrl, sessionId)),
  chatMessagesBefore: (baseUrl: string, sessionId: string, before: number) =>
    text(Console.chatMessagesBefore(baseUrl, sessionId, before)),
  chatPrompts: (baseUrl: string, sessionId: string) =>
    text(Console.chatPrompts(baseUrl, sessionId)),
  deleteSession: (baseUrl: string, sessionId: string) =>
    parsed(() => Console.deleteSession(baseUrl, sessionId)),
  restoreSession: (baseUrl: string, sessionId: string) =>
    parsed(() => Console.restoreSession(baseUrl, sessionId)),
  settleSession: (baseUrl: string, sessionId: string) =>
    parsed(() => Console.settleSession(baseUrl, sessionId)),
  unsettleSession: (baseUrl: string, sessionId: string) =>
    parsed(() => Console.unsettleSession(baseUrl, sessionId)),
  setSessionPinned: (baseUrl: string, sessionId: string, pinned: boolean) =>
    parsed(() => Console.setSessionPinned(baseUrl, sessionId, pinned)),
  forkSession: (baseUrl: string, sessionId: string, messageId: string) =>
    json<{ id: string }>(Console.forkSession(baseUrl, sessionId, messageId)),
  sessionChanges: (baseUrl: string, sessionId: string, turnIndex: number | null) =>
    text(Console.sessionChanges(baseUrl, sessionId, turnIndex)),
  sessionChangeDiff: (baseUrl: string, sessionId: string, path: string, turnIndex: number) =>
    text(Console.sessionChangeDiff(baseUrl, sessionId, path, turnIndex)),
  setChangeReviewed: (
    baseUrl: string,
    sessionId: string,
    path: string,
    turnIndex: number,
    reviewed: boolean,
  ) => parsed(() => Console.setChangeReviewed(baseUrl, sessionId, path, turnIndex, reviewed)),
  approvePermission: (baseUrl: string, sessionId: string, requestId: string, allow: boolean) =>
    parsed(() => Console.approvePermission(baseUrl, sessionId, requestId, allow)),
  answerQuestion: (baseUrl: string, sessionId: string, requestId: string, answer: unknown) =>
    parsed(() => Console.answerQuestion(baseUrl, sessionId, requestId, answer)),

  // Runs
  runPrompt: (
    baseUrl: string,
    sessionId: string,
    prompt: unknown,
    onEvent: (value: unknown) => void,
  ) => parsed(() => Console.runPrompt(baseUrl, sessionId, prompt, onEvent)),

  // Auth
  authPaired: (baseUrl: string) => Console.authPaired(baseUrl),
  authPairWithCode: (host: string, code: string, deviceName: string) =>
    text(Console.authPairWithCode(host, code, deviceName)),
  authSplitPairingLine: (line: string): { host: string; code: string } =>
    JSON.parse(Console.authSplitPairingLine(line)) as { host: string; code: string },
  activeServerUrl: (): string => Console.activeServerUrl(),
  setActiveServerUrl: (url: string) => Console.setActiveServerUrl(url),

  // Files and git
  fsBrowse: (baseUrl: string, path: string | null) => text(Console.fsBrowse(baseUrl, path)),
  fsReadFile: (baseUrl: string, path: string) => text(Console.fsReadFile(baseUrl, path)),
  fileSearch: (
    baseUrl: string,
    sessionId: string | null,
    query: string,
    root: string | null,
    includeDirs: boolean,
  ) => text(Console.fileSearch(baseUrl, sessionId, query, root, includeDirs)),
  gitStatus: (baseUrl: string, repoPath: string) => text(Console.gitStatus(baseUrl, repoPath)),
  gitDiff: (baseUrl: string, repoPath: string, filePath: string | null) =>
    text(Console.gitDiff(baseUrl, repoPath, filePath)),
  gitBranches: (baseUrl: string, repoPath: string) => text(Console.gitBranches(baseUrl, repoPath)),

  // Catalog and data (drafts, model catalog, favorites, usage, settings)
  draft: (baseUrl: string, sessionId: string) => text(Console.draft(baseUrl, sessionId)),
  saveDraft: (baseUrl: string, draft: unknown) => parsed(() => Console.saveDraft(baseUrl, draft)),
  providers: (baseUrl: string) => text(Console.providers(baseUrl)),
  providerModels: (baseUrl: string, providerId: string) =>
    text(Console.providerModels(baseUrl, providerId)),
  modelFavorites: (baseUrl: string) => text(Console.modelFavorites(baseUrl)),
  setModelFavorite: (baseUrl: string, provider: string, modelId: string, favorite: boolean) =>
    parsed(() => Console.setModelFavorite(baseUrl, provider, modelId, favorite)),
  usage: (baseUrl: string) => text(Console.usage(baseUrl)),
  settings: (baseUrl: string) => text(Console.settings(baseUrl)),

  // Workspace layout persistence (local, via the bridge)
  loadWorkspaceLayout: (key: string): Promise<string | null> => Console.loadWorkspaceLayout(key),
  saveWorkspaceLayout: (key: string, document: string): Promise<void> =>
    Console.saveWorkspaceLayout(key, document),
  deleteWorkspaceLayout: (key: string): Promise<void> => Console.deleteWorkspaceLayout(key),
};
