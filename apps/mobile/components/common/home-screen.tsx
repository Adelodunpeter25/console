import React, { useState } from "react";
import { Text, View, TextInput, TouchableOpacity, Alert, FlatList, ActivityIndicator } from "react-native";
import { useAddProject } from "@console/api";
import { Folder02Icon } from "hugeicons-react";
import { SessionSubList } from "./session-sub-list";
import { ProjectInfo } from "@console/types";

interface HomeScreenProps {
  projects: ProjectInfo[];
  refetchProjects: () => void;
  selectedProjectId: string | null;
  setSelectedProjectId: (id: string | null) => void;
  selectedSessionId: string | null;
  setSelectedSessionId: (id: string | null) => void;
  setActiveTab: (tab: "home" | "chat") => void;
}

export function HomeScreen({
  projects,
  refetchProjects,
  selectedProjectId,
  setSelectedProjectId,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
}: HomeScreenProps) {
  const [projectPathInput, setProjectPathInput] = useState("");
  const [showAddForm, setShowAddForm] = useState(false);
  const addProjectMutation = useAddProject();

  const handleAddProject = async (pathToAdd?: string) => {
    const targetPath = (pathToAdd || projectPathInput).trim();
    if (!targetPath) return;
    try {
      await addProjectMutation.mutateAsync(targetPath);
      setProjectPathInput("");
      setShowAddForm(false);
      refetchProjects();
    } catch {
      Alert.alert("Error", "Failed to add project path");
    }
  };

  return (
    <View className="flex-1 px-4 pt-3">
      {/* Header */}
      <View className="flex-row justify-between items-center mb-4">
        <View>
          <Text className="text-xl font-bold text-[#f1f3f7] tracking-tight">
            Console Workspace
          </Text>
          <Text className="text-xs text-[#9095a0] mt-0.5">
            Manage your repositories & AI agent sessions
          </Text>
        </View>

        <TouchableOpacity
          className="bg-white/10 px-3 py-1.5 rounded-lg border border-white/10"
          onPress={() => setShowAddForm(!showAddForm)}
        >
          <Text className="text-xs font-semibold text-[#38bdf8]">
            {showAddForm ? "Close" : "+ Add Project"}
          </Text>
        </TouchableOpacity>
      </View>

      {/* Add Project Card Flow (t3code inspired) */}
      {showAddForm && (
        <View className="bg-[#121316] border border-white/10 rounded-xl p-4 mb-4 shadow-lg">
          <Text className="text-sm font-semibold text-[#f1f3f7] mb-1">
            Add Local Project Folder
          </Text>
          <Text className="text-xs text-[#9095a0] mb-3">
            Enter the absolute filesystem path to your codebase directory:
          </Text>

          <View className="flex-row gap-2 mb-3">
            <TextInput
              className="flex-1 h-10 bg-[#16171a] border border-white/10 rounded-lg px-3 text-[#f1f3f7] text-xs"
              value={projectPathInput}
              onChangeText={setProjectPathInput}
              placeholder="/Users/username/projects/my-app"
              placeholderTextColor="#9095a0"
              autoCapitalize="none"
              autoCorrect={false}
            />
            <TouchableOpacity
              className={`h-10 bg-[#f1f3f7] rounded-lg px-4 items-center justify-center ${
                !projectPathInput.trim() || addProjectMutation.isPending ? "opacity-40" : ""
              }`}
              onPress={() => handleAddProject()}
              disabled={!projectPathInput.trim() || addProjectMutation.isPending}
            >
              {addProjectMutation.isPending ? (
                <ActivityIndicator size="small" color="#09090b" />
              ) : (
                <Text className="text-[#09090b] text-xs font-semibold">Confirm</Text>
              )}
            </TouchableOpacity>
          </View>
        </View>
      )}

      {/* Project Tree List */}
      {projects.length === 0 ? (
        <View className="items-center justify-center py-12 bg-[#121316]/50 rounded-xl border border-white/5 p-6">
          <Folder02Icon size={32} color="#9095a0" />
          <Text className="text-[#f1f3f7] text-sm font-medium mt-3">No Projects Added</Text>
          <Text className="text-[#9095a0] text-xs text-center mt-1 max-w-xs">
            Tap "+ Add Project" above to connect your local repository folder to Console.
          </Text>
        </View>
      ) : (
        <FlatList
          data={projects}
          keyExtractor={(item) => (item as ProjectInfo).id}
          renderItem={({ item }) => {
            const project = item as ProjectInfo;
            const isSelected = selectedProjectId === project.id;
            return (
              <View className="bg-[#121316] rounded-xl mb-2.5 border border-white/5 overflow-hidden">
                <TouchableOpacity
                  className={`flex-row items-center justify-between p-3.5 ${
                    isSelected ? "bg-white/5 border-b border-white/5" : ""
                  }`}
                  onPress={() => {
                    setSelectedProjectId(isSelected ? null : project.id);
                    setSelectedSessionId(null);
                  }}
                >
                  <View className="flex-row items-center gap-3 flex-1 pr-2">
                    <Folder02Icon size={18} color={isSelected ? "#38bdf8" : "#9095a0"} />
                    <View className="flex-1">
                      <Text
                        className={`text-sm font-semibold ${
                          isSelected ? "text-[#f1f3f7]" : "text-[#9095a0]"
                        }`}
                      >
                        {project.name}
                      </Text>
                      <Text className="text-[11px] text-[#9095a0]/70 font-mono truncate" numberOfLines={1}>
                        {project.path}
                      </Text>
                    </View>
                  </View>
                </TouchableOpacity>

                {isSelected && (
                  <SessionSubList
                    projectId={project.id}
                    projectPath={project.path}
                    selectedSessionId={selectedSessionId}
                    setSelectedSessionId={setSelectedSessionId}
                    setActiveTab={setActiveTab}
                  />
                )}
              </View>
            );
          }}
        />
      )}
    </View>
  );
}
