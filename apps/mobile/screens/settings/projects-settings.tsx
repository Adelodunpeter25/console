import React, { useEffect, useState } from "react";
import { FlatList, Pressable, Text, View, ActivityIndicator, Modal } from "react-native";
import { Folder, Trash2, Plus } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { confirmAlert, EmptyState } from "@/components/common";
import { useProjectStore } from "@/stores";
import { AddProjectScreen } from "@/screens/projects/add-project-screen";
import { theme } from "@/styles/theme";

interface ProjectsSettingsProps {
  onBack?: () => void;
}

export function ProjectsSettings({ onBack }: ProjectsSettingsProps) {
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
    confirmAlert(
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
              confirmAlert("Failed", err instanceof Error ? err.message : "Unable to remove project.");
            } finally {
              setBusyId(null);
            }
          },
        },
      ],
    );
  };

  const addFolderButton = (
    <Pressable
      className="flex-row items-center gap-1.5 px-3 py-2 rounded-full bg-card border border-border"
      style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
      onPress={() => setShowAddProject(true)}
    >
      <Plus size={15} color="#ffffff" />
      <Text className="text-xs font-semibold text-foreground">Add Folder</Text>
    </Pressable>
  );

  return (
    <View style={{ flex: 1 }}>
      <ScreenHeader
        title="Projects"
        onBack={onBack}
        rightAction={addFolderButton}
      />
      <View style={{ flex: 1, paddingHorizontal: 16, paddingTop: 4 }}>

      {loading && projects.length === 0 ? (
        <View className="items-center justify-center py-16">
          <ActivityIndicator size="small" color="#ffffff" />
          <Text className="text-xs text-foreground-secondary mt-3">Loading projects…</Text>
        </View>
      ) : projects.length === 0 ? (
        <EmptyState
          icon={<Folder size={32} color={theme.colors.text.muted} />}
          title="No project folders"
          description="Add a project folder from your host filesystem to start creating sessions."
        />
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
                  className="w-8 h-8 rounded-lg items-center justify-center bg-red-500/10 border border-red-500/20"
                  style={({ pressed }) => ({ opacity: pressed || busy ? 0.6 : 1 })}
                  hitSlop={8}
                  disabled={busy}
                  onPress={() => handleDelete(item.id, item.name)}
                >
                  {busy ? (
                    <ActivityIndicator size="small" color="#f87171" />
                  ) : (
                    <Trash2 size={14} color="#f87171" />
                  )}
                </Pressable>
              </View>
            );
          }}
        />
      )}
      </View>

      {/* Add Project Full Screen Modal */}
      <Modal
        visible={showAddProject}
        animationType="slide"
        presentationStyle="fullScreen"
        onRequestClose={() => setShowAddProject(false)}
      >
        <AddProjectScreen
          onClose={() => setShowAddProject(false)}
          onProjectAdded={() => {
            setShowAddProject(false);
            void loadProjects();
          }}
        />
      </Modal>
    </View>
  );
}

export default ProjectsSettings;

