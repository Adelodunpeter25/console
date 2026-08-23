import { useChatStore } from "@/stores/useChatStore";
import { queryClient } from "@/query-client";

/** Clear the local chat cache (in-memory + persisted). Called when the
 *  backend URL changes so stale messages from a different server don't
 *  leak into the new connection. */
export function clearChatCache() {
  try {
    useChatStore.setState({ sessions: {} });
    useChatStore.persist?.clearStorage?.();
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
}
