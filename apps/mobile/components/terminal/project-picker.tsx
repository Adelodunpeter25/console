import React from "react";
import { Pressable, Text, View } from "react-native";
import type { ProjectInfo } from "@console/types";

interface ProjectPickerProps {
  readonly projects: readonly ProjectInfo[];
  readonly onSelect: (projectId: string) => void;
}

/** Shown when the terminal has no valid project scope: tap a row to switch. */
export function ProjectPicker({ projects, onSelect }: ProjectPickerProps) {
  return (
    <View className="flex-1 items-center justify-center gap-2 px-6">
      <Text className="text-xs text-foreground-muted">Select a project for this terminal</Text>
      {projects.map((p) => (
        <Pressable
          key={p.id}
          className="w-full max-w-sm rounded-lg bg-card border border-border px-4 py-2.5 gap-0.5"
          style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
          onPress={() => onSelect(p.id)}
        >
          <Text className="text-sm font-semibold text-foreground">{p.name}</Text>
          <Text className="text-xs font-mono text-foreground-muted" numberOfLines={1}>
            {p.path}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}
