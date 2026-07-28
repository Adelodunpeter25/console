import type { AgentMessage } from "./agent.js";
import type { Model } from "./model.js";
import type { AgentTool } from "./tool.js";

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
}

export interface SessionContext {
  model: Model;
  messages: AgentMessage[];
  tools: AgentTool[];
}
