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
    <View className="flex-1 px-4 pt-3">
      {/* Workspace Header */}
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
          className="bg-sky-500/20 border border-sky-500/30 px-4 py-2 rounded-full active:opacity-80"
          onPress={() => setIsAddingProject(true)}
        >
          <Text className="text-xs font-bold text-[#38bdf8]">
            + Add Project
          </Text>
        </TouchableOpacity>
      </View>

      {/* Project Tree List */}
      {projects.length === 0 ? (
        <GlassSurface className="items-center justify-center py-12 m-2 p-6">
          <Folder size={36} color="#38bdf8" />
          <Text className="text-[#f1f3f7] text-sm font-semibold mt-3">No Projects Added</Text>
          <Text className="text-[#9095a0] text-xs text-center mt-1.5 max-w-xs leading-5">
            Tap "+ Add Project" to browse your host's filesystem directly and select a folder.
          </Text>
          <TouchableOpacity
            className="mt-4 bg-sky-500 py-2.5 px-5 rounded-full"
            onPress={() => setIsAddingProject(true)}
          >
            <Text className="text-xs font-bold text-white">Browse Filesystem</Text>
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
              <GlassSurface className="mb-3 p-0 overflow-hidden rounded-2xl">
                <TouchableOpacity
                  className={`flex-row items-center justify-between p-4 ${
                    isSelected ? "bg-white/5 border-b border-white/10" : ""
                  }`}
                  onPress={() => {
                    setSelectedProjectId(isSelected ? null : project.id);
                    setSelectedSessionId(null);
                  }}
                >
                  <View className="flex-row items-center gap-3 flex-1 pr-2">
                    <Folder size={20} color={isSelected ? "#38bdf8" : "#9095a0"} />
                    <View className="flex-1">
                      <Text
                        className={`text-sm font-semibold ${
                          isSelected ? "text-[#f1f3f7]" : "text-[#9095a0]"
                        }`}
                      >
                        {project.name}
                      </Text>
                      <Text className="text-[11px] text-[#9095a0]/70 font-mono" numberOfLines={1}>
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
