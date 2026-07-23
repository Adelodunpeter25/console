import React, { useState } from "react";
import { Text, View, TouchableOpacity, FlatList } from "react-native";
import { Folder } from "lucide-react-native";
import { SessionSubList } from "../../components/common/session-sub-list";
import { AddProjectScreen } from "../projects/add-project-screen";
import { GlassSurface } from "../../components/common/glass-surface";
import { ProjectInfo } from "@console/types";

interface HomeScreenProps {
  projects: ProjectInfo[];
  refetchProjects: () => void;
  selectedProjectId: string | null;
  setSelectedProjectId: (id: string | null) => void;
  selectedSessionId: string | null;
  setSelectedSessionId: (id: string | null) => void;
  setActiveTab: (tab: "home" | "chat" | "settings") => void;
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
  const [isAddingProject, setIsAddingProject] = useState(false);

  if (isAddingProject) {
    return (
      <AddProjectScreen
        onClose={() => setIsAddingProject(false)}
        onProjectAdded={(newId: string) => {
          setSelectedProjectId(newId);
          refetchProjects();
        }}
      />
    );
  }

  return (
    <View className="flex-1 px-4 pt-4">
      {/* Workspace Header */}
      <View className="flex-row justify-between items-center mb-5">
        <View>
          <Text className="text-2xl font-bold text-white tracking-tight">
            Console Workspace
          </Text>
          <Text className="text-sm text-zinc-400 mt-1">
            Manage your repositories & AI agent sessions
          </Text>
        </View>

        <TouchableOpacity
          className="bg-transparent border border-white/20 px-4 py-2 rounded-full active:bg-white/10"
          onPress={() => setIsAddingProject(true)}
        >
          <Text className="text-sm font-semibold text-white">
            + Add Project
          </Text>
        </TouchableOpacity>
      </View>

      {/* Project Tree List */}
      {projects.length === 0 ? (
        <GlassSurface className="items-center justify-center py-14 m-2 p-6">
          <Folder size={40} color="#ffffff" />
          <Text className="text-white text-base font-semibold mt-4">No Projects Added</Text>
          <Text className="text-zinc-400 text-sm text-center mt-2 max-w-xs leading-6">
            Tap "+ Add Project" to browse your host's filesystem directly and select a folder.
          </Text>
          <TouchableOpacity
            className="mt-6 bg-white py-3 px-6 rounded-full"
            onPress={() => setIsAddingProject(true)}
          >
            <Text className="text-sm font-bold text-black">Browse Filesystem</Text>
          </TouchableOpacity>
        </GlassSurface>
      ) : (
        <FlatList
          data={projects}
          keyExtractor={(item) => (item as ProjectInfo).id}
          renderItem={({ item }) => {
            const project = item as ProjectInfo;
            const isSelected = selectedProjectId === project.id;
            return (
              <GlassSurface className="mb-3.5 p-0 overflow-hidden rounded-2xl">
                <TouchableOpacity
                  className={`flex-row items-center justify-between p-4 ${
                    isSelected ? "bg-white/10 border-b border-white/10" : ""
                  }`}
                  onPress={() => {
                    setSelectedProjectId(isSelected ? null : project.id);
                    setSelectedSessionId(null);
                  }}
                >
                  <View className="flex-row items-center gap-3.5 flex-1 pr-2">
                    <Folder size={22} color="#ffffff" />
                    <View className="flex-1">
                      <Text
                        className={`text-base font-semibold ${
                          isSelected ? "text-white" : "text-zinc-300"
                        }`}
                      >
                        {project.name}
                      </Text>
                      <Text className="text-xs text-zinc-400 font-mono mt-0.5" numberOfLines={1}>
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
              </GlassSurface>
            );
          }}
        />
      )}
    </View>
  );
}
