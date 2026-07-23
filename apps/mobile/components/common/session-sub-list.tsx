import React from "react";
import { Text, View, TouchableOpacity, Alert } from "react-native";
import { useSessions, useCreateSession, useDeleteSession } from "@console/api";
import { styles } from "../../styles/styles";

interface SessionSubListProps {
  projectId: string;
  projectPath: string;
  selectedSessionId: string | null;
  setSelectedSessionId: (id: string | null) => void;
  setActiveTab: (tab: "home" | "chat") => void;
}

export function SessionSubList({
  projectId,
  projectPath,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
}: SessionSubListProps) {
  const { data: sessions = [], refetch: refetchSessions } = useSessions({ projectId });
  const createSessionMutation = useCreateSession();
  const deleteSessionMutation = useDeleteSession();

  const handleCreateSession = async () => {
    try {
      const sess = await createSessionMutation.mutateAsync({
        cwd: projectPath,
        projectId,
        title: "New mobile session",
      });
      setSelectedSessionId(sess.id);
      refetchSessions();
      setActiveTab("chat");
    } catch (e) {
      Alert.alert("Error", "Failed to create session");
    }
  };

  const handleDeleteSession = async (id: string) => {
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
          } catch (e) {
            Alert.alert("Error", "Failed to delete session");
          }
        },
      },
    ]);
  };

  return (
    <View style={styles.sessionsContainer}>
      <View style={styles.sessionHeaderRow}>
        <Text style={styles.sessionsTitle}>Sessions</Text>
        <TouchableOpacity style={styles.newSessionBtn} onPress={handleCreateSession}>
          <Text style={styles.newSessionBtnText}>+ New Chat</Text>
        </TouchableOpacity>
      </View>

      {sessions.map((sess) => {
        const isActive = selectedSessionId === sess.id;
        return (
          <View key={sess.id} style={[styles.sessionRow, isActive && styles.sessionRowActive]}>
            <TouchableOpacity
              style={styles.sessionClickArea}
              onPress={() => {
                setSelectedSessionId(sess.id);
                setActiveTab("chat");
              }}
            >
              <Text style={[styles.sessionText, isActive && styles.sessionTextActive]}>
                {sess.title || "New Chat"}
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.sessionDeleteBtn}
              onPress={() => handleDeleteSession(sess.id)}
            >
              <Text style={styles.sessionDeleteText}>✕</Text>
            </TouchableOpacity>
          </View>
        );
      })}

      {sessions.length === 0 && (
        <Text style={styles.emptySessionsText}>No active chat sessions.</Text>
      )}
    </View>
  );
}
