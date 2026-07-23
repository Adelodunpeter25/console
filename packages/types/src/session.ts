import type { AgentMessage } from "./agent";
import type { Model } from "./model";
import type { AgentTool } from "./tool";

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
