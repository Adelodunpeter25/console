import { useCallback, useEffect, useMemo } from "react";
import { useAppStore } from "@/stores/useAppStore";
import { useProjectStore } from "@/stores/useProjectStore";
import { useSessionStatusStore } from "@/stores/useSessionStatusStore";
import { confirmAlert } from "@/components/common/confirm-dialog";

/**
 * Project-scoped session management backed by `useProjectStore`.
 *
 * Exposes the same surface as before (sessions, create, delete, select) but
 * now runs through the desktop-parity project store so list + status state
 * stays consistent across screens.
 */
export function useProjectSessions(projectId: string, projectPath: string) {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const createSession = useProjectStore((state) => state.createSession);
  const deleteSession = useProjectStore((state) => state.deleteSession);

  // Pull the flat session list and filter to this project.
  const allSessions = useProjectStore((state) => state.sessions);
  const sessionsLoading = useProjectStore((state) => state.sessionsLoading);
  const loadSessions = useProjectStore((state) => state.loadSessions);

  const sessions = useMemo(
    () => allSessions.filter((s) => s.projectId === projectId),
    [allSessions, projectId],
  );

  // Load sessions when the project changes.
  useEffect(() => {
    if (allSessions.length === 0 && !sessionsLoading) {
      loadSessions().catch(() => {});
    }
  }, [allSessions.length, sessionsLoading, loadSessions]);

  const handleCreate = useCallback(async () => {
    try {
      const sess = await createSession(projectPath, projectId);
      setSelectedSessionId(sess.id);
      setActiveTab("chat");
    } catch {
      confirmAlert("Error", "Failed to create session");
    }
  }, [createSession, projectId, projectPath, setActiveTab, setSelectedSessionId]);

  const handleDelete = useCallback(
    async (id: string) => {
      confirmAlert("Delete", "Delete this chat session?", [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: async () => {
            try {
              await deleteSession(id);
              if (selectedSessionId === id) {
                setSelectedSessionId(null);
              }
            } catch {
              confirmAlert("Error", "Failed to delete session");
            }
          },
        },
      ]);
    },
    [deleteSession, selectedSessionId, setSelectedSessionId],
  );

  const statuses = useSessionStatusStore((state) => state.statuses);

  return {
    sessions,
    selectedSessionId,
    setSelectedSessionId,
    createSession: handleCreate,
    deleteSession: handleDelete,
    refetchSessions: loadSessions,
    isCreating: false,
    statuses,
  };
}
