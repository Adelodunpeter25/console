import { invoke } from "@tauri-apps/api/core";
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import type {
  AgentSessionEvent,
  ApprovalModeOption,
  AuthStatusResponse,
  CreateSessionDto,
  FileSearchResponse,
  ImageAttachment,
  ProjectInfo,
  ProviderCatalogEntry,
  SessionDetailResponse,
  SessionHeader,
  SlashCommandInfo,
  UpdateSessionDto,
} from "@console/types";
import type {
  BrowseResult,
  CreateDirectoryResult,
  DeleteDirectoryResult,
  DeleteFileResult,
  DirectoryTreeResult,
  LoginUrlResult,
  OAuthCallbackResult,
  PickFolderResult,
  ProviderModelsResult,
  ReadFileResult,
  WriteFileResult,
} from "../types";
import type { PickedImage } from "../types";

/**
 * Thin Tauri IPC bridge. Every method maps to a `#[tauri::command]` in
 * `src-tauri` and ultimately to a Console server route. Response types live
 * in `src/types` (desktop-specific) and `@console/types` (shared).
 */
export const tauriApi = {
  // --- server / health -----------------------------------------------------
  pingServer: () => invoke<unknown>("ping_server"),
  getBackendUrl: () => invoke<string>("get_backend_url"),
  setBackendUrl: (url: string) => invoke<void>("set_backend_url", { url }),

  // --- auth ----------------------------------------------------------------
  getAuthStatus: () => invoke<AuthStatusResponse>("get_auth_status"),
  getLoginUrl: (provider: string) => invoke<LoginUrlResult>("get_login_url", { provider }),
  handleOAuthCallback: (provider: string, code: string, state?: string) =>
    invoke<OAuthCallbackResult>("handle_oauth_callback", {
      provider,
      code,
      state,
    }),
  loginWithBrowser: (provider: string) =>
    invoke<OAuthCallbackResult>("login_with_browser", { provider }),

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
  getSession: (id: string) => invoke<SessionDetailResponse>("get_session", { id }),
  updateSession: (id: string, dto: UpdateSessionDto) =>
    invoke<SessionHeader>("update_session", {
      id,
      title: dto.title,
      cwd: dto.cwd,
      modelId: dto.modelId,
      provider: dto.provider,
      approvalMode: dto.approvalMode,
    }),
  deleteSession: (id: string) => invoke<unknown>("delete_session", { id }),

  // --- projects ------------------------------------------------------------
  listProjects: () => invoke<ProjectInfo[]>("list_projects"),
  addProject: (path: string) => invoke<ProjectInfo>("add_project", { path }),

  // --- providers / models --------------------------------------------------
  listProviders: () => invoke<ProviderCatalogEntry[]>("list_providers"),
  getProviderModels: (providerId: string) =>
    invoke<ProviderModelsResult>("get_provider_models", { providerId }),

  // --- config / approval modes --------------------------------------------
  getApprovalModes: () => invoke<ApprovalModeOption[]>("get_approval_modes"),

  // --- filesystem ----------------------------------------------------------
  browseDirectory: (path?: string) => invoke<BrowseResult>("browse_directory", { path }),
  pickFolder: () => invoke<PickFolderResult>("pick_folder"),
  getDirectoryTree: (path?: string, depth?: number) =>
    invoke<DirectoryTreeResult>("get_directory_tree", { path, depth }),
  readFile: (path: string, startLine?: number, endLine?: number) =>
    invoke<ReadFileResult>("read_file", { path, startLine, endLine }),
  writeFile: (path: string, content: string) =>
    invoke<WriteFileResult>("write_file", { path, content }),
  deleteFile: (path: string) => invoke<DeleteFileResult>("delete_file", { path }),
  createDirectory: (path: string) => invoke<CreateDirectoryResult>("create_directory", { path }),
  deleteDirectory: (path: string) => invoke<DeleteDirectoryResult>("delete_directory", { path }),

  // --- agent run / streaming -----------------------------------------------
  runAgent: (
    sessionId: string,
    prompt: string,
    modelId?: string,
    provider?: string,
    approvalMode?: string,
    attachments?: ImageAttachment[],
  ) =>
    invoke<void>("run_agent", {
      sessionId,
      prompt,
      modelId,
      provider,
      approvalMode,
      attachments,
    }),
  abortRun: (sessionId: string) => invoke<unknown>("abort_run", { sessionId }),
  answerQuestion: (sessionId: string, requestId: string, answer: string | string[]) =>
    invoke<unknown>("answer_question", {
      sessionId,
      requestId,
      answer,
    }),
  approvePermission: (sessionId: string, requestId: string, allow: boolean) =>
    invoke<unknown>("approve_permission", {
      sessionId,
      requestId,
      allow,
    }),

  listenAgentEvents: (
    sessionId: string,
    callback: (event: AgentSessionEvent) => void,
  ): Promise<UnlistenFn> =>
    listen<AgentSessionEvent>(`agent-event:${sessionId}`, (e) => callback(e.payload)),

  // --- desktop assistant (slash commands + @ file refs) --------------------
  listSlashCommands: (sessionId: string) =>
    invoke<SlashCommandInfo[]>("list_slash_commands", { sessionId }),
  searchFiles: (sessionId: string, query: string) =>
    invoke<FileSearchResponse>("search_files", { sessionId, query }),

  // --- image attachments ----------------------------------------------------
  pickImages: () => invoke<PickedImage[]>("pick_images"),
  readDroppedImages: (paths: string[]) => invoke<PickedImage[]>("read_dropped_images", { paths }),
};
