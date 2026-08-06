/**
 * Request DTOs for Hono API endpoints.
 */

export interface CreateSessionDto {
  cwd: string;
  modelId?: string;
  provider?: "gemini" | "antigravity" | "opencode";
  title?: string;
}

export interface UpdateSessionDto {
  title?: string;
  cwd?: string;
  modelId?: string;
  provider?: "gemini" | "antigravity" | "opencode";
  approvalMode?: "always-ask" | "accept-edits" | "plan-mode" | "full-access";
}

export interface RunPromptDto {
  prompt: string;
  modelId?: string;
  provider?: "gemini" | "antigravity" | "opencode";
  approvalMode?: "always-ask" | "accept-edits" | "plan-mode" | "full-access";
  /** Image attachments to include with the prompt (base64-encoded). */
  attachments?: ImageAttachment[];
}

/** An image sent with a run prompt. */
export interface ImageAttachment {
  data: string;
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

export interface AnswerQuestionDto {
  requestId: string;
  answer: string | string[];
}

export interface ApproveToolPermissionDto {
  requestId: string;
  allow: boolean;
}
