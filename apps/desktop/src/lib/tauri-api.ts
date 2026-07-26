import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import type {
  AgentSessionEvent,
  AuthStatusResponse,
  CreateSessionDto,
  FsTreeEntry,
  Model,
  ProjectInfo,
  ProviderCatalogEntry,
  SessionDetailResponse,
  SessionHeader,
  UpdateSessionDto,
} from "@console/types";

// ---------------------------------------------------------------------------
// Response shapes — mirror the `data` field returned by the Console server.
// ---------------------------------------------------------------------------

export interface BrowseResult {
  path: string;
  parentPath: string | null;
  entries: FsTreeEntry[];
}

export interface LoginUrlResult {
  provider: string;
  authUrl: string;
  redirectUri: string;
}

export interface OAuthCallbackResult {
  provider: string;
  userEmail?: string;
  projectId?: string;
}

export interface ProviderModelsResult {
  provider: string;
  models: Model[];
}

export interface PickFolderResult {
  path: string;
}

export interface DirectoryTreeResult {
  path: string;
  treeFormatted: string;
}

export interface ReadFileResult {
  path: string;
  content: string;
}

export interface WriteFileResult {
  path: string;
  message: string;
}

export interface DeleteFileResult {
  path: string;
  deleted: boolean;
}

export interface CreateDirectoryResult {
  path: string;
  created: boolean;
}

export interface DeleteDirectoryResult {
  path: string;
  deleted: boolean;
}

// ---------------------------------------------------------------------------

export const tauriApi = {
  // --- server / health -----------------------------------------------------
  pingServer: () => invoke<unknown>("ping_server"),
  getBackendUrl: () => invoke<string>("get_backend_url"),
  setBackendUrl: (url: string) => invoke<void>("set_backend_url", { url }),

  // --- auth ----------------------------------------------------------------
  getAuthStatus: () => invoke<AuthStatusResponse>("get_auth_status"),
  getLoginUrl: (provider: string) =>
    invoke<LoginUrlResult>("get_login_url", { provider }),
  handleOAuthCallback: (provider: string, code: string, state?: string) =>
    invoke<OAuthCallbackResult>("handle_oauth_callback", {
      provider,
      code,
      state,
    }),

  // --- sessions ------------------------------------------------------------
  listSessions: (cwd?: string, projectId?: string) =>
    invoke<SessionHeader[]>("list_sessions", { cwd, projectId }),
  createSession: (dto: CreateSessionDto) =>
    invoke<SessionHeader>("create_session", {
      cwd: dto.cwd,
      projectId: dto.projectId,
      modelId: dto.modelId,
      provider: dto.provider,
      title: dto.title,
    }),
  getSession: (id: string) =>
    invoke<SessionDetailResponse>("get_session", { id }),
  updateSession: (id: string, dto: UpdateSessionDto) =>
    invoke<SessionHeader>("update_session", {
      id,
      title: dto.title,
      modelId: dto.modelId,
      provider: dto.provider,
    }),
  deleteSession: (id: string) => invoke<unknown>("delete_session", { id }),

  // --- projects ------------------------------------------------------------
  listProjects: () => invoke<ProjectInfo[]>("list_projects"),
  addProject: (path: string) => invoke<ProjectInfo>("add_project", { path }),

  // --- providers / models --------------------------------------------------
  listProviders: () => invoke<ProviderCatalogEntry[]>("list_providers"),
  getProviderModels: (providerId: string) =>
    invoke<ProviderModelsResult>("get_provider_models", { providerId }),

  // --- filesystem ----------------------------------------------------------
  browseDirectory: (path?: string) =>
    invoke<BrowseResult>("browse_directory", { path }),
  pickFolder: () => invoke<PickFolderResult>("pick_folder"),
  getDirectoryTree: (path?: string, depth?: number) =>
    invoke<DirectoryTreeResult>("get_directory_tree", { path, depth }),
  readFile: (path: string, startLine?: number, endLine?: number) =>
    invoke<ReadFileResult>("read_file", { path, startLine, endLine }),
  writeFile: (path: string, content: string) =>
    invoke<WriteFileResult>("write_file", { path, content }),
  deleteFile: (path: string) =>
    invoke<DeleteFileResult>("delete_file", { path }),
  createDirectory: (path: string) =>
    invoke<CreateDirectoryResult>("create_directory", { path }),
  deleteDirectory: (path: string) =>
    invoke<DeleteDirectoryResult>("delete_directory", { path }),

  // --- agent run / streaming -----------------------------------------------
  runAgent: (
    sessionId: string,
    prompt: string,
    modelId?: string,
    provider?: string,
    approvalMode?: string,
  ) =>
    invoke<void>("run_agent", {
      sessionId,
      prompt,
      modelId,
      provider,
      approvalMode,
    }),
  abortRun: (sessionId: string) =>
    invoke<unknown>("abort_run", { sessionId }),

  listenAgentEvents: (
    sessionId: string,
    callback: (event: AgentSessionEvent) => void,
  ): Promise<UnlistenFn> =>
    listen<AgentSessionEvent>(
      `agent-event:${sessionId}`,
      (e) => callback(e.payload),
    ),
};
