import React, { useState } from "react";
import { Text, View, TextInput, TouchableOpacity, Alert } from "react-native";
import { useAddProject } from "@console/api";
import { LegendList } from "@legendapp/list";
import { Folder02Icon } from "hugeicons-react";
import { styles } from "../../styles/styles";
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
    } catch (e) {
      Alert.alert("Error", "Failed to add project path");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.headerTitle}>Console Projects</Text>

      {/* Add Project Bar */}
      <View style={styles.addProjectBar}>
        <TextInput
          style={styles.projectInput}
          value={projectPathInput}
          onChangeText={setProjectPathInput}
          placeholder="/absolute/path/to/project"
          placeholderTextColor="#9095a0"
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TouchableOpacity style={styles.projectAddBtn} onPress={handleAddProject}>
          <Text style={styles.projectAddBtnText}>Add</Text>
        </TouchableOpacity>
      </View>

      {projects.length === 0 ? (
        <View style={styles.emptyList}>
          <Text style={styles.emptyListText}>No projects configured.</Text>
        </View>
      ) : (
        <LegendList
          data={projects}
          keyExtractor={(item) => (item as ProjectInfo).id}
          estimatedItemLength={() => 60}
          renderItem={({ item }) => {
            const project = item as ProjectInfo;
            const isSelected = selectedProjectId === project.id;
            return (
              <View style={styles.projectCard}>
                <TouchableOpacity
                  style={[styles.projectHeader, isSelected && styles.projectHeaderActive]}
                  onPress={() => {
                    setSelectedProjectId(isSelected ? null : project.id);
                    setSelectedSessionId(null);
                  }}
                >
                  <Folder02Icon size={16} color={isSelected ? "#38bdf8" : "#9095a0"} />
                  <Text style={[styles.projectTitle, isSelected && styles.projectTitleActive]}>
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
