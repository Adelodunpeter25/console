import type { AgentMessage } from "./agent";
import type { Model } from "./model";
import type { AgentTool } from "./tool";

export type SessionStatus = "idle" | "working" | "done" | "needs_attention";

export interface SessionHeader {
  id: string;
  title: string;
  cwd: string;
  projectId?: string;
  modelId: string;
  provider: string;
  createdAt: number;
  updatedAt: number;
  messageCount?: number;
  status?: SessionStatus;
  approvalMode?: string;
  deletedAt?: number;
}

export interface SessionContext {
  model: Model;
  messages: AgentMessage[];
  tools: AgentTool[];
}

export interface SessionFileChange {
  path: string;
  status: "modified" | "added" | "deleted";
  additions: number;
  deletions: number;
  turnIndex: number;
  updatedAt: number;
}

