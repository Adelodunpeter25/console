import type { AgentMessage } from "./agent.js";
import type { Model } from "./model.js";
import type { AgentTool } from "./tool.js";

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
}

export interface SessionContext {
  model: Model;
  messages: AgentMessage[];
  tools: AgentTool[];
}
