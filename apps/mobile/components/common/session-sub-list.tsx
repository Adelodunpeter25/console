import React from "react";
import { Text, View, TouchableOpacity, ActivityIndicator } from "react-native";
import { formatRelativeTime } from "../../utils/time";
import { useProjectSessions } from "../../hooks";
import { theme } from "../../styles/theme";

interface SessionSubListProps {
  projectId: string;
  projectPath: string;
}

export function SessionSubList({ projectId, projectPath }: SessionSubListProps) {
  const {
    sessions,
    selectedSessionId,
    setSelectedSessionId,
    createSession,
    deleteSession,
    isCreating,
  } = useProjectSessions(projectId, projectPath);

  return (
    <View className="px-4 pb-4 pt-2 bg-foreground/5 border-t border-border">
      <View className="flex-row justify-between items-center mb-3">
        <Text className="text-xs font-semibold text-foreground-secondary uppercase tracking-wider">
          Chat Sessions ({sessions.length})
        </Text>

        <TouchableOpacity
          className={`px-3 py-1.5 border border-border rounded-full flex-row items-center gap-1.5 ${
            isCreating ? "opacity-50" : ""
          }`}
          onPress={createSession}
          disabled={isCreating}
        >
          {isCreating ? (
            <ActivityIndicator size="small" color={theme.colors.text.primary} />
          ) : (
            <Text className="text-xs font-bold text-foreground">+ New Chat</Text>
          )}
        </TouchableOpacity>
      </View>

      {sessions.length === 0 ? (
        <View className="py-3 items-center">
          <Text className="text-xs text-foreground-secondary italic">
            No active chat sessions for this project.
          </Text>
        </View>
      ) : (
        sessions.map((sess) => {
          const isSessionActive = selectedSessionId === sess.id;
          const displayTime = formatRelativeTime(sess.createdAt);

          return (
            <TouchableOpacity
              key={sess.id}
              className={`p-3 mb-2 rounded-xl border flex-row items-center justify-between ${
                isSessionActive ? "bg-foreground/15 border-foreground/30" : "bg-card border-border"
              }`}
              onPress={() => setSelectedSessionId(isSessionActive ? null : sess.id)}
            >
              <View className="flex-1 pr-3">
                <Text
                  className={`text-sm font-medium ${
                    isSessionActive ? "text-foreground font-semibold" : "text-foreground-secondary"
                  }`}
                  numberOfLines={1}
                >
                  {sess.title || "Untitled Session"}
                </Text>
                <Text
                  className="text-xs text-foreground-secondary font-mono mt-0.5"
                  numberOfLines={1}
                >
                  {sess.modelId}
                </Text>
              </View>

              <View className="flex-row items-center gap-3">
                {displayTime ? (
                  <Text className="text-xs text-foreground-secondary font-medium">
                    {displayTime}
                  </Text>
                ) : null}

                <TouchableOpacity className="p-1" onPress={() => deleteSession(sess.id)}>
                  <Text className="text-xs text-foreground-secondary">✕</Text>
                </TouchableOpacity>
              </View>
            </TouchableOpacity>
          );
        })
      )}
    </View>
  );
}
