import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import type {
  AgentSessionEvent,
  AuthStatusResponse,
  CreateSessionDto,
  FsTreeEntry,
  ProjectInfo,
  ProviderCatalogEntry,
  SessionDetailResponse,
  SessionHeader,
  UpdateSessionDto,
} from "@console/types";

export interface BrowseResult {
  path: string;
  parentPath: string | null;
  entries: FsTreeEntry[];
}

export const tauriApi = {
  pingServer: () => invoke<unknown>("ping_server"),
  getBackendUrl: () => invoke<string>("get_backend_url"),
  setBackendUrl: (url: string) => invoke<void>("set_backend_url", { url }),

  getAuthStatus: () => invoke<AuthStatusResponse>("get_auth_status"),
  getLoginUrl: (provider: string) =>
    invoke<unknown>("get_login_url", { provider }),
  handleOAuthCallback: (provider: string, code: string, state?: string) =>
    invoke<unknown>("handle_oauth_callback", { provider, code, state }),

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
  deleteSession: (id: string) =>
    invoke<unknown>("delete_session", { id }),

  listProjects: () => invoke<ProjectInfo[]>("list_projects"),
  addProject: (path: string) => invoke<ProjectInfo>("add_project", { path }),

  listProviders: () => invoke<ProviderCatalogEntry[]>("list_providers"),
  getProviderModels: (providerId: string) =>
    invoke<unknown>("get_provider_models", { providerId }),

  browseDirectory: (path?: string) =>
    invoke<BrowseResult>("browse_directory", { path }),
  pickFolder: () => invoke<unknown>("pick_folder"),
  getDirectoryTree: (path?: string, depth?: number) =>
    invoke<unknown>("get_directory_tree", { path, depth }),
  readFile: (path: string, startLine?: number, endLine?: number) =>
    invoke<unknown>("read_file", { path, startLine, endLine }),
  writeFile: (path: string, content: string) =>
    invoke<unknown>("write_file", { path, content }),
  deleteFile: (path: string) => invoke<unknown>("delete_file", { path }),
  createDirectory: (path: string) =>
    invoke<unknown>("create_directory", { path }),
  deleteDirectory: (path: string) =>
    invoke<unknown>("delete_directory", { path }),

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
