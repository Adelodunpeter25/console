import React, { useRef, useState } from "react";
import { View, ScrollView, RefreshControl } from "react-native";
import { Pencil, Trash2 } from "lucide-react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { SearchBar } from "../../components/common/search-bar";
import { confirmAlert } from "../../components/common/confirm-dialog";
import { SessionList } from "../../components/home/session-list";
import {
  SessionActionSheet,
  type SessionActionSheetHandle,
  type ActionSheetItem,
} from "../../components/context-menu/session-action-sheet";
import { useHomeSessions } from "../../hooks";
import type { SessionHeader } from "@console/types";

export function HomeScreen() {
  const {
    sections,
    searchQuery,
    setSearchQuery,
    openSession,
    composeSession,
    deleteSession,
    isCreatingSession,
    isLoadingSessions,
    isRefreshing,
    onRefresh,
    getProjectNameForSession,
    getBranchForSession,
    navigateToSettings,
  } = useHomeSessions();

  // Single shared action sheet — imperative ref + active session state
  const actionSheetRef = useRef<SessionActionSheetHandle>(null);
  const [activeSession, setActiveSession] = useState<SessionHeader | null>(null);

  const handleLongPress = (session: SessionHeader) => {
    setActiveSession(session);
    actionSheetRef.current?.open();
  };

  const handleCompose = async () => {
    try {
      await composeSession();
    } catch {
      confirmAlert("Unable to start chat", "Check the backend connection and try again.");
    }
  };

  const actionSheetItems: ActionSheetItem[] = [
    {
      key: "rename",
      label: "Rename",
      icon: <Pencil size={18} color="#a1a1aa" />,
      onPress: () => {
        confirmAlert("Rename", `Rename "${activeSession?.title || "Untitled Session"}" — coming soon`);
      },
    },
    {
      key: "delete",
      label: "Delete",
      icon: <Trash2 size={18} color="#f87171" />,
      destructive: true,
      onPress: () => {
        if (!activeSession) return;
        const targetSession = activeSession;
        confirmAlert(
          "Delete Chat",
          `Are you sure you want to delete "${targetSession.title || "Untitled Session"}"?`,
          [
            { text: "Cancel", style: "cancel" },
            {
              text: "Delete",
              style: "destructive",
              onPress: async () => {
                try {
                  await deleteSession(targetSession.id);
                } catch (err) {
                  confirmAlert("Failed", err instanceof Error ? err.message : "Unable to delete chat.");
                }
              },
            },
          ],
        );
      },
    },
  ];

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader title="Console" showSettings onSettingsPress={navigateToSettings} />

      <ScrollView
        className="flex-1"
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
        <SessionList
          sections={sections}
          searchQuery={searchQuery}
          isLoading={isLoadingSessions}
          isCreatingSession={isCreatingSession}
          openSession={openSession}
          composeSession={composeSession}
          onLongPressSession={handleLongPress}
          getProjectNameForSession={getProjectNameForSession}
          getBranchForSession={getBranchForSession}
        />
      </ScrollView>

      {/* Single shared action sheet */}
      <SessionActionSheet ref={actionSheetRef} items={actionSheetItems} />

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
