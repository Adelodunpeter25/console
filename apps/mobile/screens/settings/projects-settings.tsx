import React, { useEffect, useState } from "react";
import { Alert, FlatList, Pressable, Text, View, ActivityIndicator } from "react-native";
import { Folder, Trash2, Plus } from "lucide-react-native";
import { useProjectStore } from "../../stores";
import { AddProjectScreen } from "../projects/add-project-screen";
import { theme } from "../../styles/theme";

export function ProjectsSettings() {
  const projects = useProjectStore((state) => state.projects);
  const loading = useProjectStore((state) => state.loading);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const deleteProject = useProjectStore((state) => state.deleteProject);

  const [showAddProject, setShowAddProject] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  const handleDelete = (id: string, name: string) => {
    Alert.alert(
      "Remove Project",
      `Are you sure you want to remove "${name}" from your project workspace list? The folder on disk will not be deleted.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Remove",
          style: "destructive",
          onPress: async () => {
            setBusyId(id);
            try {
              await deleteProject(id);
            } catch (err) {
              Alert.alert("Failed", err instanceof Error ? err.message : "Unable to remove project.");
            } finally {
              setBusyId(null);
            }
          },
        },
      ],
    );
  };

  if (showAddProject) {
    return (
      <AddProjectScreen
        onClose={() => setShowAddProject(false)}
        onProjectAdded={() => {
          setShowAddProject(false);
          void loadProjects();
        }}
      />
    );
  }

  return (
    <View style={{ flex: 1, paddingHorizontal: 16, paddingTop: 8 }}>
      <View className="flex-row items-center justify-between mb-4">
        <View className="flex-1 pr-3">
          <Text className="text-sm text-foreground-secondary">
            Workspace folders tracked for agent sessions.
          </Text>
        </View>
        <Pressable
          className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-lg bg-card-alt border border-border"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={() => setShowAddProject(true)}
        >
          <Plus size={14} color="#ffffff" />
          <Text className="text-xs font-semibold text-foreground">Add Folder</Text>
        </Pressable>
      </View>

      {loading && projects.length === 0 ? (
        <View className="items-center justify-center py-16">
          <ActivityIndicator size="small" color="#ffffff" />
          <Text className="text-xs text-foreground-secondary mt-3">Loading projects…</Text>
        </View>
      ) : projects.length === 0 ? (
        <View className="items-center justify-center py-20 bg-card border border-border rounded-2xl p-6">
          <Folder size={28} color={theme.colors.text.muted} />
          <Text className="text-sm font-semibold text-foreground mt-3">No project folders</Text>
          <Text className="text-xs text-foreground-secondary text-center mt-1">
            Add a project folder from your host filesystem to start creating sessions.
          </Text>
        </View>
      ) : (
        <FlatList
          data={projects}
          keyExtractor={(item) => item.id}
          showsVerticalScrollIndicator={false}
          contentContainerStyle={{ paddingBottom: 32 }}
          renderItem={({ item }) => {
            const busy = busyId === item.id;
            return (
              <View className="flex-row items-center justify-between bg-card border border-border rounded-xl p-3.5 mb-2">
                <View className="flex-row items-center gap-3 flex-1 pr-3">
                  <View className="w-8 h-8 rounded-lg items-center justify-center bg-card-alt border border-border/60">
                    <Folder size={16} color={theme.colors.text.secondary} />
                  </View>
                  <View className="flex-1">
                    <Text className="text-sm font-semibold text-foreground" numberOfLines={1}>
                      {item.name}
                    </Text>
                    <Text className="text-xs text-foreground-secondary font-mono mt-0.5" numberOfLines={1}>
                      {item.path}
                    </Text>
                  </View>
                </View>

                <Pressable
                  className="w-8 h-8 rounded-lg items-center justify-center bg-card-alt/80 border border-border/40"
                  style={({ pressed }) => ({ opacity: pressed || busy ? 0.6 : 1 })}
                  hitSlop={8}
                  disabled={busy}
                  onPress={() => handleDelete(item.id, item.name)}
                >
                  {busy ? (
                    <ActivityIndicator size="small" color={theme.colors.status.error} />
                  ) : (
                    <Trash2 size={14} color={theme.colors.status.error} />
                  )}
                </Pressable>
              </View>
            );
          }}
        />
      )}
    </View>
  );
}

export default ProjectsSettings;
