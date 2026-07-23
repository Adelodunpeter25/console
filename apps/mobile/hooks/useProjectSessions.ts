import { useCallback } from "react";
import { Alert } from "react-native";
import { useSessions, useCreateSession, useDeleteSession } from "@console/api";
import { useAppStore } from "../stores/useAppStore";

export function useProjectSessions(projectId: string, projectPath: string) {
  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const setSelectedSessionId = useAppStore((state) => state.setSelectedSessionId);
  const setActiveTab = useAppStore((state) => state.setActiveTab);

  const { data: sessions = [], refetch: refetchSessions } = useSessions({ projectId });
  const createSessionMutation = useCreateSession();
  const deleteSessionMutation = useDeleteSession();

  const createSession = useCallback(async () => {
    try {
      const sess = await createSessionMutation.mutateAsync({
        cwd: projectPath,
        projectId,
        title: "New Chat",
      });
      setSelectedSessionId(sess.id);
      refetchSessions();
      setActiveTab("chat");
    } catch {
      Alert.alert("Error", "Failed to create session");
    }
  }, [
    createSessionMutation,
    projectId,
    projectPath,
    refetchSessions,
    setActiveTab,
    setSelectedSessionId,
  ]);

  const deleteSession = useCallback(
    async (id: string) => {
      Alert.alert("Delete", "Delete this chat session?", [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: async () => {
            try {
              await deleteSessionMutation.mutateAsync(id);
              if (selectedSessionId === id) {
                setSelectedSessionId(null);
              }
              refetchSessions();
            } catch {
              Alert.alert("Error", "Failed to delete session");
            }
          },
        },
      ]);
    },
    [deleteSessionMutation, refetchSessions, selectedSessionId, setSelectedSessionId],
  );

  return {
    sessions,
    selectedSessionId,
    setSelectedSessionId,
    createSession,
    deleteSession,
    refetchSessions,
    isCreating: createSessionMutation.isPending,
  };
}
