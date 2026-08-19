import React from "react";
import { Alert, Text, View, Pressable, ScrollView, RefreshControl } from "react-native";
import { Plus, Pencil, Trash2 } from "lucide-react-native";
import { HugeiconsIcon } from "@hugeicons/react-native";
import { Folder02Icon } from "@hugeicons/core-free-icons";
import { ScreenHeader } from "../../components/layout/screen-header";
import { SearchBar } from "../../components/common/search-bar";
import { SessionActionSheet } from "../../components/context-menu/session-context-menu";
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
    isRefreshing,
    onRefresh,
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
        refreshControl={
          <RefreshControl
            refreshing={isRefreshing}
            onRefresh={onRefresh}
            tintColor="#71717a"
            colors={["#71717a"]}
          />
        }
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
              {/* Section Header with + button */}
              <View className="flex-row justify-between items-center mb-2 px-1">
                <View className="flex-row items-center gap-1.5">
                  <HugeiconsIcon icon={Folder02Icon} size={14} color="#71717a" />
                  <Text className="text-xs font-bold text-foreground-secondary tracking-widest">
                    {section.projectName}
                  </Text>
                </View>
                <Pressable
                  className="w-6 h-6 rounded-md items-center justify-center bg-card-alt/80 border border-border/40"
                  style={({ pressed }) => ({ opacity: pressed ? 0.6 : 1 })}
                  hitSlop={8}
                  onPress={async () => {
                    try {
                      await composeSession(section.projectId);
                    } catch {
                      Alert.alert("Unable to start chat", "Check the backend connection and try again.");
                    }
                  }}
                  disabled={isCreatingSession}
                >
                  <Plus size={14} color={theme.colors.text.secondary} />
                </Pressable>
              </View>

              {/* Session Cards grouped into one card */}
              <View className="bg-card border border-border rounded-2xl overflow-hidden">
                {section.data.map((session, index) => {
                  const status = getStatusStyle(session.status);
                  const projectName = getProjectNameForSession(session);
                  const branch = getBranchForSession(session);
                  const isLast = index === section.data.length - 1;

                  return (
                    <SessionActionSheet
                      key={session.id}
                      items={[
                        {
                          key: "rename",
                          label: "Rename",
                          icon: <Pencil size={18} color="#a1a1aa" />,
                          onPress: () => {
                            // TODO: rename session
                            Alert.alert("Rename", `Rename "${session.title || "Untitled Session"}" — coming soon`);
                          },
                        },
                        {
                          key: "delete",
                          label: "Delete",
                          icon: <Trash2 size={18} color="#f87171" />,
                          destructive: true,
                          onPress: () => {
                            // TODO: delete session
                            Alert.alert("Delete", `Delete "${session.title || "Untitled Session"}" — coming soon`);
                          },
                        },
                      ]}
                    >
                      {(onLongPress) => (
                        <Pressable
                          className={`flex-row items-center px-4 py-3.5 ${!isLast ? "border-b border-border/40" : ""}`}
                          style={({ pressed }) => ({ opacity: pressed ? 0.65 : 1 })}
                          onPress={() => openSession(session.id)}
                          onLongPress={onLongPress}
                          delayLongPress={350}
                        >
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
                      )}
                    </SessionActionSheet>
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
