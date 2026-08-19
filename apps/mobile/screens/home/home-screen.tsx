import React from "react";
import { Alert, Text, View, Pressable, ScrollView } from "react-native";
import { Folder, MessageSquare } from "lucide-react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { SearchBar } from "../../components/common/search-bar";
import { useHomeSessions } from "../../hooks";
import { formatRelativeTime } from "../../utils/time";
import { theme } from "../../styles/theme";

function getStatusStyle(status?: string): {
  label: string;
  color: string;
  bgColor: string;
} {
  switch (status) {
    case "working":
      return {
        label: "Running",
        color: theme.colors.status.running,
        bgColor: theme.colors.status.runningBg,
      };
    case "done":
      return {
        label: "Ready",
        color: theme.colors.status.ready,
        bgColor: theme.colors.status.readyBg,
      };
    case "needs_attention":
      return {
        label: "Attention",
        color: theme.colors.status.attention,
        bgColor: theme.colors.status.attentionBg,
      };
    default:
      return {
        label: "Idle",
        color: theme.colors.status.idle,
        bgColor: theme.colors.status.idleBg,
      };
  }
}

function shortRelativeTime(dateInput?: number): string {
  const full = formatRelativeTime(dateInput);
  return full.replace(" ago", "").replace("just now", "now");
}

export function HomeScreen() {
  const {
    sections,
    searchQuery,
    setSearchQuery,
    openSession,
    composeSession,
    isCreatingSession,
    getProjectNameForSession,
    getBranchForSession,
    navigateToSettings,
  } = useHomeSessions();

  const handleCompose = async () => {
    try {
      await composeSession();
    } catch {
      Alert.alert("Unable to start chat", "Check the backend connection and try again.");
    }
  };

  return (
    <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
      <ScreenHeader title="Console" showSettings onSettingsPress={navigateToSettings} />

      <ScrollView
        style={{ flex: 1 }}
        contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 32 }}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        showsVerticalScrollIndicator={false}
      >
        {sections.length === 0 ? (
          <View className="items-center justify-center py-20">
            <Text className="text-foreground-secondary text-sm">
              {searchQuery ? "No matching sessions" : "No sessions yet"}
            </Text>
          </View>
        ) : (
          sections.map((section) => (
            <View key={section.projectId ?? section.projectName} className="mb-5">
              {/* Section Header */}
              <View className="flex-row justify-between items-center mb-2 px-1">
                <View className="flex-row items-center gap-1.5">
                  <Folder size={14} color="#71717a" />
                  <Text className="text-xs font-bold text-foreground-secondary tracking-widest">
                    {section.projectName}
                  </Text>
                </View>
                <Text className="text-xs text-foreground-secondary opacity-60">
                  {section.data.length} {section.data.length === 1 ? "chat" : "chats"}
                </Text>
              </View>

              {/* Session Cards grouped into one card */}
              <View className="bg-card border border-border rounded-2xl overflow-hidden">
                {section.data.map((session, index) => {
                  const status = getStatusStyle(session.status);
                  const projectName = getProjectNameForSession(session);
                  const branch = getBranchForSession(session);
                  const isLast = index === section.data.length - 1;

                  return (
                    <Pressable
                      key={session.id}
                      className={`flex-row items-center px-4 py-3.5 ${!isLast ? "border-b border-border/40" : ""}`}
                      style={({ pressed }) => ({ opacity: pressed ? 0.65 : 1 })}
                      onPress={() => openSession(session.id)}
                    >
                      {/* Chat message avatar icon with status colour */}
                      <View
                        className="w-9 h-9 rounded-xl items-center justify-center mr-3"
                        style={{ backgroundColor: status.bgColor }}
                      >
                        <MessageSquare size={16} color={status.color} />
                      </View>

                      {/* Title + project/branch */}
                      <View className="flex-1 mr-2">
                        <Text
                          className="text-sm font-semibold text-foreground mb-0.5"
                          numberOfLines={1}
                        >
                          {session.title || "Untitled Session"}
                        </Text>
                        <View className="flex-row items-center gap-1">
                          <Text className="text-xs text-foreground-secondary">{projectName}</Text>
                          {branch ? (
                            <>
                              <Text className="text-xs text-foreground-secondary">•</Text>
                              <Text className="text-xs text-foreground-secondary">{branch}</Text>
                            </>
                          ) : null}
                        </View>
                      </View>

                      {/* Status badge + time */}
                      <View className="items-end gap-1.5">
                        <View
                          className="px-2 py-0.5 rounded-full"
                          style={{ backgroundColor: status.bgColor }}
                        >
                          <Text
                            className="text-[9px] font-bold tracking-wide"
                            style={{ color: status.color }}
                          >
                            {status.label}
                          </Text>
                        </View>
                        <Text className="text-[10px] text-foreground-secondary">
                          {shortRelativeTime(session.updatedAt ?? session.createdAt)}
                        </Text>
                      </View>
                    </Pressable>
                  );
                })}
              </View>
            </View>
          ))
        )}
      </ScrollView>

      {/* Sticky search bar */}
      <SearchBar
        value={searchQuery}
        onChangeText={setSearchQuery}
        onComposePress={handleCompose}
        disabled={isCreatingSession}
      />
    </View>
  );
}

export default HomeScreen;
