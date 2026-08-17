import type {
  AgentSessionEvent,
  ApprovalModeOption,
  AuthStatusResponse,
  CreateSessionDto,
  FileSearchResponse,
  GitStatusSummary,
  ImageAttachment,
  ModelFavorite,
  ProjectInfo,
  ProviderCatalogEntry,
  SessionDetailResponse,
  SessionHeader,
  SlashCommandInfo,
  TerminalSpawnedEvent,
  TerminalServerMessage,
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
  PickedImage,
} from "../types";

export type UnlistenFn = () => void;

let backendUrl = localStorage.getItem("console_backend_url") || "http://localhost:3000";

// Terminal WebSockets registry
const activeTerminals = new Map<string, WebSocket>();
const terminalListeners = new Map<string, Set<(msg: TerminalServerMessage) => void>>();
const agentListeners = new Map<string, Set<(event: AgentSessionEvent) => void>>();
const fsListeners = new Set<() => void>();

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const url = `${backendUrl}${path.startsWith("/") ? path : `/${path}`}`;
  const res = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const errText = await res.text().catch(() => "Unknown error");
    throw new Error(`HTTP ${res.status}: ${errText}`);
  }

  const json = await res.json();
  if (json && typeof json === "object" && "success" in json) {
    if (!json.success) {
      throw new Error(json.error || "API returned failure");
    }
    return json.data as T;
  }
  return json as T;
}

declare global {
  interface Window {
    electronApi?: {
      confirmDialog: (title: string, message: string) => Promise<boolean>;
      pickFolder: () => Promise<string | null>;
      pickImages: () => Promise<PickedImage[]>;
      readDroppedImages: (paths: string[]) => Promise<PickedImage[]>;
      openExternal: (url: string) => Promise<void>;
      showNotification: (title: string, body: string) => Promise<void>;
      getAppVersion: () => Promise<string>;
      authLoginWithBrowser: (opts: {
        provider: string;
        authUrl: string;
        port?: number;
        callbackPath?: string;
      }) => Promise<{ code: string }>;
    };
  }
}

/**
 * Desktop API client connecting directly to the Console backend and Electron native bridge.
 */
export const api = {
  // --- server / health ---
  pingServer: () => request<unknown>("/api/health"),
  getBackendUrl: async () => backendUrl,
  setBackendUrl: async (url: string) => {
    backendUrl = url;
    localStorage.setItem("console_backend_url", url);
  },

  // --- auth ---
  getAuthStatus: () => request<AuthStatusResponse>("/api/auth/status"),
  getLoginUrl: (provider: string) =>
    request<LoginUrlResult>("/api/auth/login/url", {
      method: "POST",
      body: JSON.stringify({ provider }),
    }),
  handleOAuthCallback: (provider: string, code: string, state?: string) =>
    request<OAuthCallbackResult>("/api/auth/login/callback", {
      method: "POST",
      body: JSON.stringify({ provider, code, state }),
    }),
  loginWithBrowser: async (provider: string): Promise<OAuthCallbackResult> => {
    const res = await api.getLoginUrl(provider);
    if (!res?.authUrl) {
      throw new Error("Failed to obtain OAuth login URL from server.");
    }

    if (window.electronApi?.authLoginWithBrowser) {
      // Electron launches a local callback server, opens the browser, and waits for redirect
      const { code } = await window.electronApi.authLoginWithBrowser({
        provider,
        authUrl: res.authUrl,
      });

      // Complete token exchange on the server
      return api.handleOAuthCallback(provider, code);
    }

    // Fallback: open external browser
    if (window.electronApi?.openExternal) {
      await window.electronApi.openExternal(res.authUrl);
    }
    return { provider, userEmail: undefined, projectId: undefined };
  },
  loginCodebuff: async (): Promise<OAuthCallbackResult> => {
    const startRes = await request<{
      provider: string;
      loginUrl: string;
      fingerprintId: string;
      fingerprintHash: string;
      expiresAt: string;
    }>("/api/auth/codebuff/start", { method: "POST" });

    if (window.electronApi?.openExternal && startRes.loginUrl) {
      await window.electronApi.openExternal(startRes.loginUrl);
    }

    // Poll for status
    const params = new URLSearchParams({
      fingerprintId: startRes.fingerprintId,
      fingerprintHash: startRes.fingerprintHash,
      expiresAt: startRes.expiresAt,
    });

    while (Date.now() < Number(startRes.expiresAt) * 1000) {
      await new Promise((r) => setTimeout(r, 2000));
      try {
        const poll = await request<{ completed: boolean; email?: string }>(
          `/api/auth/codebuff/status?${params.toString()}`,
        );
        if (poll.completed) {
          return { provider: "codebuff", userEmail: poll.email };
        }
      } catch {}
    }

    throw new Error("Codebuff login timed out.");
  },
  getProjectId: (provider: string) =>
    request<{ projectId?: string }>(`/api/auth/project-id/${encodeURIComponent(provider)}`),
  setProjectId: (provider: string, projectId?: string) =>
    request<void>("/api/auth/project-id", {
      method: "POST",
      body: JSON.stringify({ provider, projectId }),
    }),

  // --- sessions ---
  listSessions: (cwd?: string, projectId?: string, onlyDeleted?: boolean) => {
    const params = new URLSearchParams();
    if (cwd) params.set("cwd", cwd);
    if (projectId) params.set("projectId", projectId);
    if (onlyDeleted) params.set("onlyDeleted", "true");
    return request<SessionHeader[]>(`/api/sessions?${params.toString()}`);
  },
  createSession: (dto: CreateSessionDto) =>
    request<SessionHeader>("/api/sessions", {
      method: "POST",
      body: JSON.stringify(dto),
    }),
  getSession: (id: string) => request<SessionDetailResponse>(`/api/sessions/${encodeURIComponent(id)}`),
  updateSession: (id: string, dto: UpdateSessionDto) =>
    request<SessionHeader>(`/api/sessions/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(dto),
    }),
  deleteSession: (id: string) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(id)}`, { method: "DELETE" }),
  restoreSession: (id: string) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(id)}/restore`, { method: "POST" }),
  permanentlyDeleteSession: (id: string) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(id)}/permanent`, { method: "DELETE" }),

  // --- projects ---
  listProjects: () => request<ProjectInfo[]>("/api/projects"),
  addProject: (path: string) =>
    request<ProjectInfo>("/api/projects", {
      method: "POST",
      body: JSON.stringify({ path }),
    }),

  // --- providers / models ---
  listProviders: () => request<ProviderCatalogEntry[]>("/api/providers"),
  getProviderModels: (providerId: string) =>
    request<ProviderModelsResult>(`/api/providers/${encodeURIComponent(providerId)}/models`),
  listModelFavorites: () => request<ModelFavorite[]>("/api/model-favorites"),
  setModelFavorite: (favorite: ModelFavorite, isFavorite: boolean) =>
    request<{ provider: string; modelId: string; favorite: boolean }>("/api/model-favorites", {
      method: "PUT",
      body: JSON.stringify({ ...favorite, favorite: isFavorite }),
    }),

  // --- config / approval modes ---
  getApprovalModes: () => request<ApprovalModeOption[]>("/api/config/approval-modes"),

  // --- git ---
  getGitStatus: (path?: string) => {
    const q = path ? `?path=${encodeURIComponent(path)}` : "";
    return request<GitStatusSummary>(`/api/git/status${q}`);
  },

  // --- filesystem ---
  browseDirectory: (path?: string) => {
    const q = path ? `?path=${encodeURIComponent(path)}` : "";
    return request<BrowseResult>(`/api/fs/browse${q}`);
  },
  pickFolder: async (): Promise<PickFolderResult> => {
    if (window.electronApi?.pickFolder) {
      const selected = await window.electronApi.pickFolder();
      return { path: selected || "" };
    }
    return { path: "" };
  },
  getDirectoryTree: (path?: string, depth?: number) => {
    const params = new URLSearchParams();
    if (path) params.set("path", path);
    if (depth) params.set("depth", String(depth));
    return request<DirectoryTreeResult>(`/api/fs/tree?${params.toString()}`);
  },
  readFile: (path: string, startLine?: number, endLine?: number) => {
    const params = new URLSearchParams({ path });
    if (startLine) params.set("startLine", String(startLine));
    if (endLine) params.set("endLine", String(endLine));
    return request<ReadFileResult>(`/api/fs/file?${params.toString()}`);
  },
  writeFile: (path: string, content: string) =>
    request<WriteFileResult>("/api/fs/file", {
      method: "POST",
      body: JSON.stringify({ path, content }),
    }),
  deleteFile: (path: string) =>
    request<DeleteFileResult>(`/api/fs/file?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    }),
  createDirectory: (path: string) =>
    request<CreateDirectoryResult>("/api/fs/dir", {
      method: "POST",
      body: JSON.stringify({ path }),
    }),
  deleteDirectory: (path: string) =>
    request<DeleteDirectoryResult>(`/api/fs/dir?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    }),
  watchDirectory: async (path: string) => {
    try {
      const url = `${backendUrl}/api/fs/watch?path=${encodeURIComponent(path)}`;
      const eventSource = new EventSource(url);
      eventSource.onmessage = () => {
        fsListeners.forEach((cb) => cb());
      };
      eventSource.onerror = () => {
        // Handled silently to avoid noisy console spam when backend restarts
      };
    } catch {}
  },
  listenFsChanges: async (callback: () => void): Promise<UnlistenFn> => {
    fsListeners.add(callback);
    return () => fsListeners.delete(callback);
  },

  // --- agent run / streaming ---
  runAgent: async (
    sessionId: string,
    prompt: string,
    modelId?: string,
    provider?: string,
    approvalMode?: string,
    attachments?: ImageAttachment[],
  ) => {
    const url = `${backendUrl}/api/sessions/${encodeURIComponent(sessionId)}/run`;
    const response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ prompt, modelId, provider, approvalMode, attachments }),
    });

    if (!response.ok) {
      const errText = await response.text().catch(() => "Unknown error");
      throw new Error(`HTTP ${response.status}: ${errText}`);
    }

    const reader = response.body?.getReader();
    if (!reader) return;

    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const lines = buffer.split("\n\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.trim()) continue;
        const dataMatch = line.match(/^data:\s*(.*)$/m);
        if (dataMatch) {
          try {
            const event = JSON.parse(dataMatch[1]) as AgentSessionEvent;
            const listeners = agentListeners.get(sessionId);
            listeners?.forEach((cb) => cb(event));
          } catch {}
        }
      }
    }
  },

  abortRun: (sessionId: string) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(sessionId)}/abort`, { method: "POST" }),
  answerQuestion: (sessionId: string, requestId: string, answer: string | string[]) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(sessionId)}/answer`, {
      method: "POST",
      body: JSON.stringify({ requestId, answer }),
    }),
  approvePermission: (sessionId: string, requestId: string, allow: boolean) =>
    request<unknown>(`/api/sessions/${encodeURIComponent(sessionId)}/approve`, {
      method: "POST",
      body: JSON.stringify({ requestId, allow }),
    }),

  listenAgentEvents: async (
    sessionId: string,
    callback: (event: AgentSessionEvent) => void,
  ): Promise<UnlistenFn> => {
    let set = agentListeners.get(sessionId);
    if (!set) {
      set = new Set();
      agentListeners.set(sessionId, set);
    }
    set.add(callback);
    return () => {
      const current = agentListeners.get(sessionId);
      current?.delete(callback);
      if (current && current.size === 0) {
        agentListeners.delete(sessionId);
      }
    };
  },

  // --- desktop assistant ---
  listSlashCommands: (sessionId: string) => {
    const encoded = encodeURIComponent(sessionId || "");
    return request<SlashCommandInfo[]>(sessionId ? `/api/assist/${encoded}/commands` : `/api/assist/commands`);
  },
  searchFiles: (sessionId: string, query: string) => {
    const encoded = encodeURIComponent(sessionId || "");
    return request<FileSearchResponse>(
      sessionId
        ? `/api/assist/${encoded}/search?q=${encodeURIComponent(query)}`
        : `/api/assist/search?q=${encodeURIComponent(query)}`,
    );
  },

  // --- images ---
  pickImages: async (): Promise<PickedImage[]> => {
    if (window.electronApi?.pickImages) {
      return window.electronApi.pickImages();
    }
    return [];
  },
  readDroppedImages: async (paths: string[]): Promise<PickedImage[]> => {
    if (window.electronApi?.readDroppedImages) {
      return window.electronApi.readDroppedImages(paths);
    }
    return [];
  },

  // --- dialogs ---
  confirmDialog: async (title: string, message: string): Promise<boolean> => {
    if (window.electronApi?.confirmDialog) {
      return window.electronApi.confirmDialog(title, message);
    }
    return window.confirm(`${title}\n\n${message}`);
  },

  // --- terminals ---
  terminalOpen: async (
    cwd: string,
    opts?: { shell?: string; cols?: number; rows?: number; label?: string },
  ): Promise<TerminalSpawnedEvent> => {
    const wsBase = backendUrl.replace(/^http/, "ws");
    const params = new URLSearchParams({
      cwd,
      cols: String(opts?.cols || 80),
      rows: String(opts?.rows || 24),
    });
    if (opts?.shell) params.set("shell", opts.shell);

    const ws = new WebSocket(`${wsBase}/api/terminals?${params.toString()}`);

    return new Promise((resolve, reject) => {
      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data) as TerminalServerMessage;
          if (msg.type === "spawned") {
            const spawnedMsg = msg as TerminalSpawnedEvent;
            activeTerminals.set(spawnedMsg.id, ws);
            resolve({
              type: "spawned",
              id: spawnedMsg.id,
              pid: spawnedMsg.pid,
              shell: spawnedMsg.shell,
              cwd: spawnedMsg.cwd,
              cols: spawnedMsg.cols,
              rows: spawnedMsg.rows,
            });
          }
          if (msg.type === "output" || msg.type === "exit" || msg.type === "error" || msg.type === "spawned") {
            terminalListeners.forEach((set) => {
              set.forEach((cb) => cb(msg));
            });
          }
        } catch (err) {
          console.error("Failed to parse terminal message", err);
        }
      };

      ws.onerror = (err) => {
        reject(err);
      };

      ws.onclose = () => {
        // clean up
      };
    });
  },

  terminalInput: async (id: string, data: string) => {
    const ws = activeTerminals.get(id);
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "input", data }));
    }
  },

  terminalResize: async (id: string, cols: number, rows: number) => {
    const ws = activeTerminals.get(id);
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "resize", cols, rows }));
    }
  },

  terminalKill: async (id: string) => {
    const ws = activeTerminals.get(id);
    if (ws) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "kill" }));
      }
      ws.close();
      activeTerminals.delete(id);
      terminalListeners.delete(id);
    }
  },

  listenTerminalEvents: async (
    terminalId: string,
    callback: (message: TerminalServerMessage) => void,
  ): Promise<UnlistenFn> => {
    let set = terminalListeners.get(terminalId);
    if (!set) {
      set = new Set();
      terminalListeners.set(terminalId, set);
    }
    set.add(callback);
    return () => {
      const current = terminalListeners.get(terminalId);
      current?.delete(callback);
      if (current && current.size === 0) {
        terminalListeners.delete(terminalId);
      }
    };
  },
};

// Aliases for backwards compatibility
export const desktopApi = api;
export const tauriApi = api;
