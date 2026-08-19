import React, { useRef, useEffect } from "react";
import { View, Text, Pressable, ScrollView, ActivityIndicator } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Folder, Check } from "lucide-react-native";
import type { ProjectInfo } from "@console/types";
import { SharedBottomSheet } from "../../common/shared-bottom-sheet";
import { useProjectStore } from "../../../stores";
import { theme } from "../../../styles/theme";

interface ProjectPickerSheetProps {
  projects: ProjectInfo[];
  selectedId: string | null;
  fallbackLabel?: string;
  onSelect: (project: ProjectInfo) => void;
}

export function ProjectPickerSheet({
  projects,
  selectedId,
  fallbackLabel = "Select Folder",
  onSelect,
}: ProjectPickerSheetProps) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);
  const loadProjects = useProjectStore((state) => state.loadProjects);
  const loadingProjects = useProjectStore((state) => state.loading);

  useEffect(() => {
    if (projects.length === 0) {
      void loadProjects();
    }
  }, [projects.length, loadProjects]);

  const selectedProject = projects.find((p) => p.id === selectedId);
  const displayLabel = selectedProject?.name ?? fallbackLabel;

  return (
    <>
      <Pressable
        className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-lg bg-card-alt/70 border border-border/50 shrink-0"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={() => {
          if (projects.length === 0) void loadProjects();
          bottomSheetRef.current?.present();
        }}
      >
        <Folder size={13} color={theme.colors.text.secondary} />
        <Text className="text-xs font-medium text-foreground">
          {displayLabel}
        </Text>
      </Pressable>

      <SharedBottomSheet ref={bottomSheetRef} title="Working Directory" snapPoints={["50%"]}>
        <ScrollView showsVerticalScrollIndicator={false}>
          {loadingProjects && projects.length === 0 ? (
            <View className="items-center justify-center py-10">
              <ActivityIndicator size="small" color={theme.colors.text.muted} />
            </View>
          ) : (
            projects.map((project) => {
              const isSelected = project.id === selectedId;
              return (
                <Pressable
                  key={project.id}
                  className={`flex-row items-center justify-between px-3.5 py-3 rounded-xl mb-1.5 ${
                    isSelected ? "bg-card-alt border border-border/60" : ""
                  }`}
                  style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                  onPress={() => {
                    onSelect(project);
                    bottomSheetRef.current?.dismiss();
                  }}
                >
                  <View className="flex-1 mr-2">
                    <Text className="text-sm font-semibold text-foreground mb-0.5">{project.name}</Text>
                    <Text className="text-xs text-foreground-secondary" numberOfLines={1}>
                      {project.path}
                    </Text>
                  </View>
                  {isSelected ? <Check size={16} color={theme.colors.status.ready} /> : null}
                </Pressable>
              );
            })
          )}
        </ScrollView>
      </SharedBottomSheet>
    </>
  );
}
