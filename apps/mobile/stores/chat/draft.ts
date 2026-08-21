/**
 * Draft helpers — isolated from main chat store so draft policy is
 * centralized and easily tested. User asked for separate file.
 *
 * - Draft = unsent input + attachments for a session (keyed by sessionId)
 * - Never-sent new chat (0 messages but has draft) becomes a "DRAFT" header
 *   on home instead of disappearing on restart.
 * - Image drafts capped to MAX_DRAFT_IMAGES = 2 per chat.
 */

import type { ImageAttachment, SessionHeader } from "@console/types";
import type { ChatSessionState } from "../../types/chat";

/** Max image attachments kept in a draft (most-recent 2). */
export const MAX_DRAFT_IMAGES = 2;

/** Draft is non-empty input or at least one attachment. */
export function isDraftSession(state: ChatSessionState): boolean {
  return state.input.trim().length > 0 || state.attachments.length > 0;
}

export function hasPersistableDraft(state: ChatSessionState): boolean {
  return isDraftSession(state);
}

/** Keep only the most recent MAX_DRAFT_IMAGES attachments. */
export function trimDraftAttachments(attachments: ImageAttachment[]): ImageAttachment[] {
  if (attachments.length <= MAX_DRAFT_IMAGES) return attachments;
  return attachments.slice(-MAX_DRAFT_IMAGES);
}

/** One-line preview for home list: "Draft: hello world" truncated. */
export function draftPreview(state: ChatSessionState, maxLen = 48): string {
  const text = state.input.trim().replace(/\s+/g, " ");
  if (text) {
    return text.length > maxLen ? `Draft: ${text.slice(0, maxLen)}…` : `Draft: ${text}`;
  }
  if (state.attachments.length > 0) {
    const n = state.attachments.length;
    return n === 1 ? "Draft: 📎 1 image" : `Draft: 📎 ${n} images`;
  }
  return "Draft";
}

/** Synthetic header for a local-only draft that has no server session yet.
 *  Used when composeSession hasn't created a SessionHeader but input already exists.
 *  Home renders this under the DRAFT project group.
 */
export function createEphemeralDraftHeader(
  id: string,
  draft: ChatSessionState,
  projectId?: string | null,
  cwd?: string,
): SessionHeader {
  const now = draft.draftUpdatedAt ?? Date.now();
  return {
    id,
    title: draftPreview(draft, 32).replace(/^Draft:\s*/, "") || "Draft",
    cwd: cwd ?? "",
    projectId: projectId ?? undefined,
    modelId: "",
    provider: "",
    createdAt: now,
    updatedAt: now,
    messageCount: 0,
    status: "idle" as const,
  };
}

/** Should this persisted session count as DRAFT section (0 messages + has draft). */
export function isDraftHeader(session: SessionHeader, state?: ChatSessionState): boolean {
  if (!state) return false;
  return (session.messageCount ?? 0) === 0 && isDraftSession(state);
}
