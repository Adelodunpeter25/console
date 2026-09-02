import { chat$ } from "@/stores/useChatStore";
import { clearChatStorage } from "@/stores/chat/chat-persist";
import { queryClient } from "@/query-client";
import { clearProjectState } from "@/stores/useProjectStore";
import { clearProviderState } from "@/stores/useProviderStore";
import { invalidateUsage } from "@/stores/useUsageStore";
import { clearSessions } from "@/stores/useSessionStore";
import { clearAllStatuses } from "@/stores/useSessionStatusStore";
import { clearFsState } from "@/stores/useFsStore";
import { clearAllTerminals } from "@/stores/useTerminalStore";
import { clearAppSelections } from "@/stores/useAppStore";

/** Clear the local chat cache (in-memory + persisted). Called when the
 *  backend URL changes so stale messages from a different server don't
 *  leak into the new connection. */
export function clearChatCache() {
  try {
    chat$.sessions.set({});
    clearChatStorage();
  } catch (err) {
    console.warn("Could not clear persisted chat storage:", err);
  }
}

/** Drop every piece of server-scoped client state. Must run whenever the
 *  active backend changes so sessions/messages/providers from the old
 *  server never render against the new one. */
export function resetServerState() {
  clearChatCache();
  queryClient.clear();
  clearProjectState();
  clearProviderState();
  invalidateUsage();
  clearSessions();
  clearAllStatuses();
  clearFsState();
  clearAllTerminals();
  clearAppSelections();
}
