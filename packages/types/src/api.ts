import type { AgentMessage } from "./agent";
import type { SessionHeader } from "./session";

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
}

export interface CreateSessionDto {
  cwd: string;
  projectId?: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
  title?: string;
}

export interface UpdateSessionDto {
  title?: string;
  cwd?: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
  approvalMode?: "always-ask" | "accept-edits" | "plan-mode" | "full-access";
}

export interface RunPromptDto {
  prompt: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
  approvalMode?: "always-ask" | "accept-edits" | "plan-mode" | "full-access";
  /** Image attachments to include with the prompt (base64-encoded). */
  attachments?: ImageAttachment[];
}

/** An image sent with a run prompt. */
export interface ImageAttachment {
  /** Base64-encoded image data (no data: prefix). */
  data: string;
  /** MIME type, e.g. "image/png". */
  mimeType: string;
}

export interface OAuthLoginUrlDto {
  provider: "gemini" | "antigravity";
}

export interface OAuthCallbackDto {
  provider: "gemini" | "antigravity";
  code: string;
  state?: string;
}

export interface ProjectIdDto {
  provider: "gemini" | "antigravity";
  projectId?: string;
}

export interface AnswerQuestionDto {
  requestId: string;
  answer: string | string[];
}

export interface ApproveToolPermissionDto {
  requestId: string;
  allow: boolean;
}

/** A slash command exposed to the desktop for autocomplete + execution. */
export interface SlashCommandInfo {
  name: string;
  description: string;
  /** true for built-ins like /model; false for discovered custom commands/skills. */
  builtin: boolean;
}

/** One file result from the FFF-backed fuzzy search. */
export interface FileSearchResult {
  /** Path relative to the search root (the session's working directory). */
  relativePath: string;
  /** Absolute path to the file. */
  absolutePath: string;
  isDir: boolean;
  score: number;
}

export interface FileSearchResponse {
  root: string;
  query: string;
  items: FileSearchResult[];
}

export interface ProjectInfo {
  id: string;
  name: string;
  path: string;
  createdAt: number;
  updatedAt: number;
}

export interface AuthStatusResponse {
  gemini: { loggedIn: boolean; email?: string; projectId?: string; configuredProjectId?: string };
  antigravity: { loggedIn: boolean; email?: string; projectId?: string; configuredProjectId?: string };
}

export interface SessionDetailResponse {
  header: SessionHeader;
  messages: AgentMessage[];
}

export interface FsTreeEntry {
  name: string;
  path: string;
  isDir: boolean;
  size?: number;
  children?: FsTreeEntry[];
}
