import React from "react";
import { Text, View, TouchableOpacity, Alert } from "react-native";
import { useSessions, useCreateSession, useDeleteSession } from "@console/api";

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
    } catch {
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
          } catch {
            Alert.alert("Error", "Failed to delete session");
          }
        },
      },
    ]);
  };

  return (
    <View className="px-3.5 pb-3.5 pt-1 border-t border-white/5">
      <View className="flex-row justify-between items-center mb-2 mt-1">
        <Text className="text-[11px] font-bold text-[#9095a0] uppercase tracking-wider">
          Sessions
        </Text>
        <TouchableOpacity
          className="py-1 px-2 rounded bg-white/10"
          onPress={handleCreateSession}
        >
          <Text className="text-[#f1f3f7] text-[10px] font-semibold">+ New Chat</Text>
        </TouchableOpacity>
      </View>

      {sessions.map((sess) => {
        const isActive = selectedSessionId === sess.id;
        return (
          <View
            key={sess.id}
            className={`flex-row items-center justify-between py-2 px-2.5 rounded-md mb-1 ${
              isActive ? "bg-white/10" : "bg-white/5"
            }`}
          >
            <TouchableOpacity
              className="flex-1 pr-2.5"
              onPress={() => {
                setSelectedSessionId(sess.id);
                setActiveTab("chat");
              }}
            >
              <Text
                className={`text-xs ${
                  isActive ? "text-[#f1f3f7] font-medium" : "text-[#9095a0]"
                }`}
              >
                {sess.title || "New Chat"}
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              className="p-1"
              onPress={() => handleDeleteSession(sess.id)}
            >
              <Text className="text-red-500 text-xs font-bold">✕</Text>
            </TouchableOpacity>
          </View>
        );
      })}

      {sessions.length === 0 && (
        <Text className="text-[11px] text-[#9095a0] italic text-center py-2">
          No active chat sessions.
        </Text>
      )}
    </View>
  );
}
