import React, { useState } from "react";
import { Text, View, Pressable, FlatList, ActivityIndicator, ScrollView } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useFsBrowse, useAddProject } from "../../hooks";
import { Folder, FolderUp, ChevronRight, Check } from "lucide-react-native";
import { ScreenHeader } from "../../components/layout/screen-header";
import { confirmAlert, EmptyState } from "../../components/common";
import { theme } from "../../styles/theme";

interface AddProjectScreenProps {
  onClose: () => void;
  onProjectAdded: (projectId: string) => void;
}

export function AddProjectScreen({ onClose, onProjectAdded }: AddProjectScreenProps) {
  const insets = useSafeAreaInsets();
  const [currentPath, setCurrentPath] = useState<string | undefined>(undefined);
  const { data: browseData, isLoading, isError, refetch } = useFsBrowse(currentPath);
  const addProjectMutation = useAddProject();

  const activePath =
    browseData?.currentPath || browseData?.path || currentPath || "";
  const parentPath = browseData?.parentPath;
  const entries = browseData?.entries || [];

  // Filter only directories for project selection
  const directories = entries.filter((e) => e.isDir);

  const handleConfirmAdd = async () => {
    if (!activePath) return;
    try {
      const added = await addProjectMutation.mutateAsync(activePath);
      onProjectAdded(added.id);
      onClose();
    } catch (e) {
      confirmAlert(
        "Failed to Add Project",
        e instanceof Error ? e.message : "Directory could not be added as a project.",
      );
    }
  };

  // Build breadcrumb segments with cumulative path
  const pathParts = activePath.split("/").filter(Boolean);
  const breadcrumbs: Array<{ name: string; path: string }> = [
    { name: "/", path: "/" },
    ...pathParts.map((part, index) => ({
      name: part,
      path: "/" + pathParts.slice(0, index + 1).join("/"),
    })),
  ];

  const currentFolder = pathParts[pathParts.length - 1] || "/";

  return (
    <View style={{ flex: 1, backgroundColor: "#0a0a0b" }}>
      {/* Shared Screen Header */}
      <ScreenHeader title="Add Project" onBack={onClose} />

      {/* Horizontal Path Breadcrumbs */}
      <View className="border-b border-border bg-card/60 px-4 py-2.5">
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ alignItems: "center", gap: 4 }}
        >
          {breadcrumbs.map((crumb, idx) => {
            const isLast = idx === breadcrumbs.length - 1;
            return (
              <React.Fragment key={crumb.path}>
                <Pressable
                  onPress={() => !isLast && setCurrentPath(crumb.path)}
                  className={`px-2 py-1 rounded-md ${
                    isLast ? "bg-card-alt border border-border/80" : ""
                  }`}
                  style={({ pressed }) => ({ opacity: pressed && !isLast ? 0.6 : 1 })}
                  disabled={isLast}
                >
                  <Text
                    className={`text-xs font-mono ${
                      isLast ? "text-foreground font-semibold" : "text-foreground-secondary"
                    }`}
                  >
                    {crumb.name}
                  </Text>
                </Pressable>
                {!isLast && <ChevronRight size={12} color={theme.colors.text.muted} />}
              </React.Fragment>
            );
          })}
        </ScrollView>
      </View>

      {/* Directory Content List */}
      <View className="flex-1 px-4 pt-3">
        {isLoading ? (
          <View className="flex-1 items-center justify-center">
            <ActivityIndicator size="large" color="#ffffff" />
            <Text className="text-xs text-foreground-secondary mt-3">Reading host directories…</Text>
          </View>
        ) : isError ? (
          <View className="flex-1 items-center justify-center p-6">
            <Text className="text-sm font-semibold text-red-400 mb-2">
              Failed to load directory
            </Text>
            <Pressable
              className="px-4 py-2 bg-card-alt border border-border rounded-xl"
              onPress={() => refetch()}
            >
              <Text className="text-xs text-foreground font-medium">Retry</Text>
            </Pressable>
          </View>
        ) : (
          <FlatList
            data={directories}
            keyExtractor={(item) => item.path}
            showsVerticalScrollIndicator={false}
            contentContainerStyle={{ paddingBottom: 16 }}
            ListHeaderComponent={
              parentPath !== null && parentPath !== undefined ? (
                <Pressable
                  className="flex-row items-center gap-3 p-3.5 mb-2 bg-card-alt/40 border border-border/40 rounded-xl"
                  style={({ pressed }) => ({ opacity: pressed ? 0.65 : 1 })}
                  onPress={() => setCurrentPath(parentPath)}
                >
                  <View className="w-8 h-8 rounded-lg items-center justify-center bg-card-alt border border-border/60">
                    <FolderUp size={16} color={theme.colors.text.secondary} />
                  </View>
                  <View className="flex-1">
                    <Text className="text-xs font-semibold text-foreground">.. (Parent Directory)</Text>
                    <Text className="text-[10px] text-foreground-secondary font-mono mt-0.5" numberOfLines={1}>
                      {parentPath}
                    </Text>
                  </View>
                </Pressable>
              ) : null
            }
            ListEmptyComponent={
              <EmptyState
                icon={<Folder size={28} color="#71717a" />}
                title="No subdirectories"
                description="There are no folders in this directory."
              />
            }
            renderItem={({ item }) => (
              <Pressable
                className="flex-row items-center justify-between p-3.5 mb-2 bg-card border border-border/60 rounded-xl"
                style={({ pressed }) => ({ opacity: pressed ? 0.65 : 1 })}
                onPress={() => setCurrentPath(item.path)}
              >
                <View className="flex-row items-center gap-3 flex-1 pr-2">
                  <View className="w-8 h-8 rounded-lg items-center justify-center bg-card-alt/80 border border-border/40">
                    <Folder size={16} color={theme.colors.text.secondary} />
                  </View>
                  <Text className="text-sm font-medium text-foreground flex-1" numberOfLines={1}>
                    {item.name}
                  </Text>
                </View>
                <ChevronRight size={15} color={theme.colors.text.muted} />
              </Pressable>
            )}
          />
        )}
      </View>

      {/* Sticky Bottom Action Bar */}
      <View
        className="px-4 pt-3 bg-card border-t border-border flex-row items-center justify-between"
        style={{ paddingBottom: Math.max(insets.bottom, 16) }}
      >
        <View className="flex-1 pr-3">
          <Text className="text-[10px] font-bold text-foreground-secondary uppercase tracking-wider">
            Selected Folder
          </Text>
          <Text className="text-sm font-semibold text-foreground mt-0.5" numberOfLines={1}>
            {currentFolder}
          </Text>
          <Text className="text-[10px] text-foreground-secondary font-mono mt-0.5" numberOfLines={1}>
            {activePath}
          </Text>
        </View>

        <Pressable
          className={`py-3 px-5 rounded-full bg-foreground items-center justify-center flex-row gap-1.5 ${
            addProjectMutation.isPending ? "opacity-50" : ""
          }`}
          style={({ pressed }) => ({ opacity: pressed && !addProjectMutation.isPending ? 0.8 : 1 })}
          onPress={handleConfirmAdd}
          disabled={addProjectMutation.isPending}
        >
          {addProjectMutation.isPending ? (
            <ActivityIndicator size="small" color="#000000" />
          ) : (
            <>
              <Check size={14} color="#000000" />
              <Text className="text-xs font-bold text-black tracking-wide">Add Folder</Text>
            </>
          )}
        </Pressable>
      </View>
    </View>
  );
}

export default AddProjectScreen;

