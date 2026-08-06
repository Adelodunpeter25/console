/**
 * Request DTOs for Hono API endpoints.
 * Shared shapes come from @console/types; this module re-exports them so
 * route/services imports stay local.
 */
export type {
  AnswerQuestionDto,
  ApproveToolPermissionDto,
  CreateSessionDto,
  ImageAttachment,
  OAuthCallbackDto,
  OAuthLoginUrlDto,
  RunPromptDto,
  UpdateSessionDto,
} from "@console/types";
