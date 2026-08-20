import React from "react";
import { View, ScrollView } from "react-native";
import type { ApprovalMode, ProjectInfo, ProviderId } from "@console/types";
import { ProjectPickerSheet } from "./selectors/project-picker-sheet";
import { ModelPickerSheet } from "./selectors/model-picker-sheet";
import { ApprovalModePickerSheet } from "./selectors/approval-mode-picker-sheet";

interface ComposerBottomStripProps {
  projects: ProjectInfo[];
  selectedProjectId: string | null;
  onProjectChange: (project: ProjectInfo) => void;
  projectLocked?: boolean;

  selectedModel: string | null;
  selectedProvider?: string | null;
  onModelChange: (modelId: string, provider?: ProviderId) => void;

  approvalMode: ApprovalMode;
  onApprovalModeChange: (mode: ApprovalMode) => void;
}

export function ComposerBottomStrip({
  projects,
  selectedProjectId,
  onProjectChange,
  projectLocked,
  selectedModel,
  selectedProvider,
  onModelChange,
  approvalMode,
  onApprovalModeChange,
}: ComposerBottomStripProps) {
  return (
    <View className="pt-2 pb-1">
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: 16, gap: 8 }}
      >
        <ProjectPickerSheet
          projects={projects}
          selectedId={selectedProjectId}
          onSelect={onProjectChange}
          locked={projectLocked}
        />
        <ModelPickerSheet
          value={selectedModel}
          provider={selectedProvider}
          onChange={onModelChange}
        />
        <ApprovalModePickerSheet
          value={approvalMode}
          onChange={onApprovalModeChange}
        />
      </ScrollView>
    </View>
  );
}
