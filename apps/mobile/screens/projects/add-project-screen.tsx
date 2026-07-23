import React, { useState } from "react";
import {
  Text,
  View,
  TouchableOpacity,
  FlatList,
  ActivityIndicator,
  Alert,
} from "react-native";
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
        e instanceof Error ? e.message : "Directory could not be added as a project."
      );
    }
  };

  const pathSegments = activePath.split("/").filter(Boolean);

  return (
    <SafeAreaView className="flex-1 bg-[#0a0a0b]" edges={["top", "bottom", "left", "right"]}>
      {/* Screen Header with Back Button */}
      <View className="px-4 py-3 border-b border-white/10 flex-row items-center justify-between bg-[#0d0d0e]">
        <TouchableOpacity
          className="flex-row items-center gap-1.5 py-1.5 px-3.5 rounded-full bg-white/10 border border-white/10"
          onPress={onClose}
        >
          <Text className="text-xs font-semibold text-[#f1f3f7]">← Back</Text>
        </TouchableOpacity>

        <Text className="text-sm font-bold text-[#f1f3f7] tracking-tight">
          Browse Host Filesystem
        </Text>

        <View className="w-16" />
      </View>

      {/* Path Breadcrumb Bar */}
      <View className="px-4 py-3 bg-[#121316]/60 border-b border-white/5 flex-row items-center gap-2">
        <Folder size={16} color="#38bdf8" />
        <Text className="text-xs font-mono text-[#38bdf8] flex-1" numberOfLines={1}>
          {activePath}
        </Text>
      </View>

      {/* Directory Content List */}
      <View className="flex-1 px-4 pt-2">
        {isLoading ? (
          <View className="flex-1 items-center justify-center">
            <ActivityIndicator size="large" color="#38bdf8" />
            <Text className="text-xs text-[#9095a0] mt-3">Reading host directories...</Text>
          </View>
        ) : isError ? (
          <View className="flex-1 items-center justify-center p-6">
            <Text className="text-sm font-semibold text-red-400 mb-2">Failed to load directory</Text>
            <TouchableOpacity
              className="px-4 py-2 bg-white/10 rounded-full"
              onPress={() => refetch()}
            >
              <Text className="text-xs text-[#f1f3f7]">Retry</Text>
            </TouchableOpacity>
          </View>
        ) : (
          <FlatList
            data={directories}
            keyExtractor={(item) => item.path}
            ListHeaderComponent={
              parentPath !== null && parentPath !== undefined ? (
                <TouchableOpacity
                  className="flex-row items-center gap-3 p-3.5 mb-2 bg-white/5 border border-white/10 rounded-xl"
                  onPress={() => setCurrentPath(parentPath)}
                >
                  <Text className="text-base">⬆️</Text>
                  <View>
                    <Text className="text-xs font-semibold text-[#38bdf8]">.. (Parent Directory)</Text>
                    <Text className="text-[10px] text-[#9095a0] font-mono">{parentPath}</Text>
                  </View>
                </TouchableOpacity>
              ) : null
            }
            ListEmptyComponent={
              <View className="items-center justify-center py-12">
                <Text className="text-xs text-[#9095a0] italic">
                  No subdirectories found in this folder.
                </Text>
              </View>
            }
            renderItem={({ item }) => (
              <TouchableOpacity
                className="flex-row items-center justify-between p-3.5 mb-2 bg-[#121316] border border-white/5 rounded-xl active:bg-white/10"
                onPress={() => setCurrentPath(item.path)}
              >
                <View className="flex-row items-center gap-3 flex-1 pr-2">
                  <Folder size={18} color="#9095a0" />
                  <Text className="text-xs font-medium text-[#f1f3f7] flex-1" numberOfLines={1}>
                    {item.name}
                  </Text>
                </View>
                <Text className="text-xs text-[#9095a0]">›</Text>
              </TouchableOpacity>
            )}
          />
        )}
      </View>

      {/* Floating Liquid-Glass Confirm Action Bar */}
      <GlassSurface className="m-4 mt-2 p-3 rounded-full bg-[#121316]/95 border-white/15 shadow-2xl flex-row items-center justify-between">
        <View className="flex-1 pr-3">
          <Text className="text-[10px] font-bold text-[#9095a0] uppercase tracking-wider">
            Selected Folder
          </Text>
          <Text className="text-xs font-semibold text-[#f1f3f7]" numberOfLines={1}>
            {pathSegments[pathSegments.length - 1] || activePath}
          </Text>
        </View>

        <TouchableOpacity
          className={`py-3 px-5 rounded-full bg-sky-500 items-center justify-center flex-row gap-2 ${
            addProjectMutation.isPending ? "opacity-50" : ""
          }`}
          onPress={handleConfirmAdd}
          disabled={addProjectMutation.isPending}
        >
          {addProjectMutation.isPending ? (
            <ActivityIndicator size="small" color="#ffffff" />
          ) : (
            <Text className="text-xs font-bold text-white tracking-wide">
              + Add Directory
            </Text>
          )}
        </TouchableOpacity>
      </GlassSurface>
    </SafeAreaView>
  );
}
