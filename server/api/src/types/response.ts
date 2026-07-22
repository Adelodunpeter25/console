/**
 * Response DTOs for Hono API endpoints.
 */
import type { AgentMessage, SessionHeader } from "../../../agent/src/types/index.js";

export interface ApiResponse<T = unknown> {
  success: boolean;
  data?: T;
  error?: string;
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
