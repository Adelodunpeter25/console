import React, { useEffect, useState } from "react";
import { FlatList, Pressable, Text, View, ActivityIndicator } from "react-native";
import { RotateCcw, Trash2, MessageSquare } from "lucide-react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { confirmAlert } from "../../components/common/confirm-dialog";
import { useProjectStore } from "../../stores";
import { formatRelativeTime, folderName } from "../../utils";
import { theme } from "../../styles/theme";

interface DeletedChatsSettingsProps {
  onBack?: () => void;
}

export function DeletedChatsSettings({ onBack }: DeletedChatsSettingsProps) {
  const deletedSessions = useProjectStore((state) => state.deletedSessions);
  const loading = useProjectStore((state) => state.deletedSessionsLoading);
  const loadDeletedSessions = useProjectStore((state) => state.loadDeletedSessions);
  const restoreSession = useProjectStore((state) => state.restoreSession);
  const permanentlyDeleteSession = useProjectStore((state) => state.permanentlyDeleteSession);

  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    void loadDeletedSessions();
  }, [loadDeletedSessions]);

  const handleRestore = async (id: string) => {
    setBusyId(id);
    try {
      await restoreSession(id);
    } catch (err) {
      confirmAlert("Failed", err instanceof Error ? err.message : "Unable to restore chat.");
    } finally {
      setBusyId(null);
    }
  };

  const handlePermanentDelete = (id: string, title: string) => {
    confirmAlert(
      "Delete Chat Permanently",
      `"${title}" and its message history will be permanently deleted. This cannot be undone.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: async () => {
            setBusyId(id);
            try {
              await permanentlyDeleteSession(id);
            } catch (err) {
              confirmAlert("Failed", err instanceof Error ? err.message : "Unable to delete chat.");
            } finally {
              setBusyId(null);
            }
          },
        },
      ],
    );
  };

  const handleRestoreAll = () => {
    if (deletedSessions.length === 0) return;
    confirmAlert(
      "Restore All Chats",
      `Restore all ${deletedSessions.length} deleted chats back to your workspace?`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Restore All",
          onPress: async () => {
            setBusyId("all");
            try {
              for (const s of deletedSessions) {
                await restoreSession(s.id);
              }
            } catch (err) {
              confirmAlert("Failed", err instanceof Error ? err.message : "Unable to restore all chats.");
            } finally {
              setBusyId(null);
            }
          },
        },
      ],
    );
  };

  const handlePermanentDeleteAll = () => {
    if (deletedSessions.length === 0) return;
    confirmAlert(
      "Delete All Chats Permanently",
      `All ${deletedSessions.length} deleted chats and their entire message history will be permanently removed. This cannot be undone.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete All Permanently",
          style: "destructive",
          onPress: async () => {
            setBusyId("all");
            try {
              for (const s of deletedSessions) {
                await permanentlyDeleteSession(s.id);
              }
            } catch (err) {
              confirmAlert("Failed", err instanceof Error ? err.message : "Unable to permanently delete chats.");
            } finally {
              setBusyId(null);
            }
          },
        },
      ],
    );
  };

  const headerActions = deletedSessions.length > 0 ? (
    <View className="flex-row items-center gap-2">
      <Pressable
        className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
        style={({ pressed }) => ({
          opacity: pressed || busyId !== null ? 0.5 : 1,
        })}
        disabled={busyId !== null}
        onPress={handleRestoreAll}
        hitSlop={6}
      >
        {busyId === "all" ? (
          <ActivityIndicator size="small" color="#ffffff" style={{ transform: [{ scale: 0.75 }] }} />
        ) : (
          <RotateCcw size={17} color="#ffffff" />
        )}
      </Pressable>

      <Pressable
        className="w-10 h-10 rounded-full bg-red-500/10 border border-red-500/25 items-center justify-center"
        style={({ pressed }) => ({
          opacity: pressed || busyId !== null ? 0.5 : 1,
        })}
        disabled={busyId !== null}
        onPress={handlePermanentDeleteAll}
        hitSlop={6}
      >
        <Trash2 size={17} color="#f87171" />
      </Pressable>
    </View>
  ) : null;

  return (
    <View style={{ flex: 1 }}>
      <ScreenHeader
        title="Deleted Chats"
        onBack={onBack}
        rightAction={headerActions}
      />

      <View style={{ flex: 1, paddingHorizontal: 16, paddingTop: 4 }}>
        {loading && deletedSessions.length === 0 ? (
          <View className="items-center justify-center py-16">
            <ActivityIndicator size="small" color="#ffffff" />
            <Text className="text-xs text-foreground-secondary mt-3">Loading deleted chats…</Text>
          </View>
        ) : deletedSessions.length === 0 ? (
          <View className="items-center justify-center py-20 bg-card border border-border rounded-2xl p-6">
            <MessageSquare size={28} color={theme.colors.text.muted} />
            <Text className="text-sm font-semibold text-foreground mt-3">No deleted chats</Text>
            <Text className="text-xs text-foreground-secondary text-center mt-1">
              Chats you delete will appear here until permanently purged.
            </Text>
          </View>
        ) : (
        <FlatList
          data={deletedSessions}
          keyExtractor={(item) => item.id}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 32 }}
          renderItem={({ item }) => {
            const busy = busyId === item.id;
            const title = item.title || "Untitled Chat";
            return (
              <View className="bg-card border border-border rounded-xl p-3.5 mb-2">
                <View className="flex-row items-center justify-between mb-2">
                  <View className="flex-1 pr-2">
                    <Text className="text-sm font-semibold text-foreground" numberOfLines={1}>
                      {title}
                    </Text>
                    <Text className="text-[11px] text-foreground-secondary mt-0.5" numberOfLines={1}>
                      {folderName(item.cwd)} · Deleted {formatRelativeTime(item.deletedAt ?? item.updatedAt)}
                    </Text>
                  </View>
                </View>

                <View className="flex-row items-center justify-end gap-2 pt-2 border-t border-border/40">
                  <Pressable
                    className="flex-row items-center gap-1 px-3 py-1.5 rounded-lg bg-card-alt border border-border/80"
                    style={({ pressed }) => ({ opacity: pressed || busy ? 0.6 : 1 })}
                    disabled={busy}
                    onPress={() => handleRestore(item.id)}
                  >
                    {busy ? (
                      <ActivityIndicator size="small" color="#ffffff" style={{ transform: [{ scale: 0.7 }] }} />
                    ) : (
                      <RotateCcw size={12} color="#ffffff" />
                    )}
                    <Text className="text-xs font-semibold text-foreground">Restore</Text>
                  </Pressable>

                  <Pressable
                    className="flex-row items-center gap-1 px-3 py-1.5 rounded-lg bg-red-500/10 border border-red-500/30"
                    style={({ pressed }) => ({ opacity: pressed || busy ? 0.6 : 1 })}
                    disabled={busy}
                    onPress={() => handlePermanentDelete(item.id, title)}
                  >
                    <Trash2 size={12} color={theme.colors.status.error} />
                    <Text className="text-xs font-semibold text-red-400">Delete</Text>
                  </Pressable>
                </View>
              </View>
            );
          }}
        />
      )}
      </View>
    </View>
  );
}

export default DeletedChatsSettings;
