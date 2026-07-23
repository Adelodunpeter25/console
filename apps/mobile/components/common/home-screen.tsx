import React, { useState } from "react";
import { Text, View, TextInput, TouchableOpacity, Alert, FlatList } from "react-native";
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
  const addProjectMutation = useAddProject();

  const handleAddProject = async () => {
    if (!projectPathInput.trim()) return;
    try {
      await addProjectMutation.mutateAsync(projectPathInput.trim());
      setProjectPathInput("");
      refetchProjects();
    } catch {
      Alert.alert("Error", "Failed to add project path");
    }
  };

  return (
    <View className="flex-1 px-4 pt-3">
      <Text className="text-xl font-bold text-[#f1f3f7] mb-4 tracking-tight">
        Console Projects
      </Text>

      {/* Add Project Bar */}
      <View className="flex-row gap-2 mb-4">
        <TextInput
          className="flex-1 h-10 bg-[#16171a] border border-white/10 rounded-lg px-3 text-[#f1f3f7] text-xs"
          value={projectPathInput}
          onChangeText={setProjectPathInput}
          placeholder="/absolute/path/to/project"
          placeholderTextColor="#9095a0"
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TouchableOpacity
          className="h-10 bg-[#f1f3f7] rounded-lg px-4 items-center justify-center"
          onPress={handleAddProject}
        >
          <Text className="text-[#09090b] text-xs font-semibold">Add</Text>
        </TouchableOpacity>
      </View>

      {projects.length === 0 ? (
        <View className="items-center justify-center py-10">
          <Text className="text-[#9095a0] text-xs italic">No projects configured.</Text>
        </View>
      ) : (
        <FlatList
          data={projects}
          keyExtractor={(item) => (item as ProjectInfo).id}
          renderItem={({ item }) => {
            const project = item as ProjectInfo;
            const isSelected = selectedProjectId === project.id;
            return (
              <View className="bg-[#121316] rounded-lg mb-2 border border-white/5 overflow-hidden">
                <TouchableOpacity
                  className={`flex-row items-center gap-3 p-3.5 ${
                    isSelected ? "bg-white/5" : ""
                  }`}
                  onPress={() => {
                    setSelectedProjectId(isSelected ? null : project.id);
                    setSelectedSessionId(null);
                  }}
                >
                  <Folder02Icon size={16} color={isSelected ? "#38bdf8" : "#9095a0"} />
                  <Text
                    className={`text-sm font-medium ${
                      isSelected ? "text-[#f1f3f7]" : "text-[#9095a0]"
                    }`}
                  >
                    {project.name}
                  </Text>
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
