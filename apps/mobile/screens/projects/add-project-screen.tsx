import React, { useState } from "react";
import { Text, View, TouchableOpacity, FlatList, ActivityIndicator, Alert } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useFsBrowse, useAddProject } from "@console/api";
import { Folder } from "lucide-react-native";
import { GlassSurface } from "../../components/common/glass-surface";

interface AddProjectScreenProps {
  onClose: () => void;
  onProjectAdded: (projectId: string) => void;
}

export function AddProjectScreen({ onClose, onProjectAdded }: AddProjectScreenProps) {
  const [currentPath, setCurrentPath] = useState<string | undefined>(undefined);
  const { data: browseData, isLoading, isError, refetch } = useFsBrowse(currentPath);
  const addProjectMutation = useAddProject();

  const activePath = browseData?.currentPath || "/";
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
      Alert.alert(
        "Failed to Add Project",
        e instanceof Error ? e.message : "Directory could not be added as a project.",
      );
    }
  };

  const pathSegments = activePath.split("/").filter(Boolean);

  return (
    <SafeAreaView className="flex-1 bg-[#0a0a0b]" edges={["top", "bottom", "left", "right"]}>
      {/* Screen Header with Back Button */}
      <View className="px-4 py-3.5 border-b border-white/10 flex-row items-center justify-between bg-[#0d0d0e]">
        <TouchableOpacity
          className="flex-row items-center gap-1.5 py-2 px-4 rounded-full bg-transparent border border-white/20"
          onPress={onClose}
        >
          <Text className="text-sm font-semibold text-white">← Back</Text>
        </TouchableOpacity>

        <Text className="text-base font-bold text-white tracking-tight">
          Browse Host Filesystem
        </Text>

        <View className="w-16" />
      </View>

      {/* Path Breadcrumb Bar */}
      <View className="px-4 py-3.5 bg-[#121316]/60 border-b border-white/10 flex-row items-center gap-2.5">
        <Folder size={18} color="#ffffff" />
        <Text className="text-sm font-mono text-white flex-1" numberOfLines={1}>
          {activePath}
        </Text>
      </View>

      {/* Directory Content List */}
      <View className="flex-1 px-4 pt-3">
        {isLoading ? (
          <View className="flex-1 items-center justify-center">
            <ActivityIndicator size="large" color="#ffffff" />
            <Text className="text-sm text-zinc-400 mt-3">Reading host directories...</Text>
          </View>
        ) : isError ? (
          <View className="flex-1 items-center justify-center p-6">
            <Text className="text-base font-semibold text-red-400 mb-2">
              Failed to load directory
            </Text>
            <TouchableOpacity
              className="px-5 py-2.5 bg-white/10 border border-white/20 rounded-full"
              onPress={() => refetch()}
            >
              <Text className="text-sm text-white font-medium">Retry</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <FlatList
            data={directories}
            keyExtractor={(item) => item.path}
            ListHeaderComponent={
              parentPath !== null && parentPath !== undefined ? (
                <TouchableOpacity
                  className="flex-row items-center gap-3.5 p-4 mb-2.5 bg-white/5 border border-white/15 rounded-xl"
                  onPress={() => setCurrentPath(parentPath)}
                >
                  <Text className="text-lg">⬆️</Text>
                  <View>
                    <Text className="text-sm font-semibold text-white">.. (Parent Directory)</Text>
                    <Text className="text-xs text-zinc-400 font-mono mt-0.5">{parentPath}</Text>
                  </View>
                </TouchableOpacity>
              ) : null
            }
            ListEmptyComponent={
              <View className="items-center justify-center py-12">
                <Text className="text-sm text-zinc-400 italic">
                  No subdirectories found in this folder.
                </Text>
              </View>
            }
            renderItem={({ item }) => (
              <TouchableOpacity
                className="flex-row items-center justify-between p-4 mb-2.5 bg-[#121316] border border-white/10 rounded-xl active:bg-white/10"
                onPress={() => setCurrentPath(item.path)}
              >
                <View className="flex-row items-center gap-3.5 flex-1 pr-2">
                  <Folder size={20} color="#ffffff" />
                  <Text className="text-base font-medium text-white flex-1" numberOfLines={1}>
                    {item.name}
                  </Text>
                </View>
                <Text className="text-sm text-zinc-400">›</Text>
              </TouchableOpacity>
            )}
          />
        )}
      </View>

      {/* Floating Liquid-Glass Confirm Action Bar */}
      <GlassSurface className="m-4 mt-2 p-3.5 rounded-full bg-[#121316]/95 border-white/20 shadow-2xl flex-row items-center justify-between">
        <View className="flex-1 pr-3">
          <Text className="text-xs font-bold text-zinc-400 uppercase tracking-wider">
            Selected Folder
          </Text>
          <Text className="text-sm font-semibold text-white mt-0.5" numberOfLines={1}>
            {pathSegments[pathSegments.length - 1] || activePath}
          </Text>
        </View>

        <TouchableOpacity
          className={`py-3.5 px-6 rounded-full bg-white items-center justify-center flex-row gap-2 ${
            addProjectMutation.isPending ? "opacity-50" : ""
          }`}
          onPress={handleConfirmAdd}
          disabled={addProjectMutation.isPending}
        >
          {addProjectMutation.isPending ? (
            <ActivityIndicator size="small" color="#000000" />
          ) : (
            <Text className="text-sm font-bold text-black tracking-wide">+ Add Directory</Text>
          )}
        </TouchableOpacity>
      </GlassSurface>
    </SafeAreaView>
  );
}
