/**
 * Request DTOs for Hono API endpoints.
 */

export interface CreateSessionDto {
  cwd: string;
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
