import type { AgentMessage } from "./agent.js";
import type { SessionHeader } from "./session.js";

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
}

export interface CreateSessionDto {
  cwd: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
  title?: string;
}

export interface UpdateSessionDto {
  title?: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
}

export interface RunPromptDto {
  prompt: string;
  modelId?: string;
  provider?: "gemini" | "antigravity";
  approvalMode?: "always-ask" | "accept-edits" | "plan-mode" | "full-access";
}

export interface OAuthLoginUrlDto {
  provider: "gemini" | "antigravity";
}

export interface OAuthCallbackDto {
  provider: "gemini" | "antigravity";
  code: string;
  state?: string;
}

export interface AnswerQuestionDto {
  requestId: string;
  answer: string | string[];
}

export interface ApproveToolPermissionDto {
  requestId: string;
  allow: boolean;
}

export interface ProjectInfo {
  name: string;
  path: string;
  lastModified?: number;
}

export interface AuthStatusResponse {
  gemini: { loggedIn: boolean; email?: string; projectId?: string };
  antigravity: { loggedIn: boolean; email?: string; projectId?: string };
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
