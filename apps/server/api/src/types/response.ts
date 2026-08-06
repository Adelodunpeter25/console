/**
 * Response DTOs for Hono API endpoints.
 * Shared shapes come from @console/types; this module adds server-local extras.
 */
import type { AgentMessage, SessionHeader } from "@console/types";

export type { ApiResponse, AuthStatusResponse, FsTreeEntry, SessionDetailResponse } from "@console/types";

export interface ProjectInfo {
  name: string;
  path: string;
  lastModified?: number;
}
