import React, { memo } from "react";
import { ActivityIndicator, Pressable, View, Text } from "react-native";
import { ChevronDown, ChevronRight, Folder, FolderOpen } from "lucide-react-native";
import { FileIcon } from "@/components/icons/file-icon";
import { theme } from "@/styles/theme";
import type { FsTreeEntry } from "@console/types";

/** One flattened row of the lazy file tree. */
export interface TreeRow {
  readonly key: string;
  readonly kind: "entry" | "loading" | "empty";
  readonly entry?: FsTreeEntry;
  readonly depth: number;
  /** Relative-to-root path (used by search results display). */
  readonly relativePath?: string;
}

export function LoadingRow({ depth }: { depth: number }): React.JSX.Element {
  return (
    <View className="min-h-[36px] flex-row items-center gap-2" style={{ paddingLeft: 8 + depth * 18 }}>
      <ActivityIndicator size="small" color={theme.colors.text.muted} />
      <Text className="text-xs text-foreground-muted">Loading…</Text>
    </View>
  );
}

export function EmptyFolderRow({ depth }: { depth: number }): React.JSX.Element {
  return (
    <View style={{ paddingLeft: 8 + depth * 18 }} className="py-1">
      <Text className="text-xs italic text-foreground-tertiary">(empty)</Text>
    </View>
  );
}

export const FileTreeRow = memo(function FileTreeRow(props: {
  readonly entry: FsTreeEntry;
  readonly depth: number;
  readonly selected: boolean;
  readonly expanded: boolean;
  readonly onPressDirectory: (path: string) => void;
  readonly onPressFile: (path: string) => void;
}) {
  const isDir = props.entry.isDir;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={props.entry.path}
      onPress={() => {
        if (isDir) props.onPressDirectory(props.entry.path);
        else props.onPressFile(props.entry.path);
      }}
      className={`mx-2 min-h-[42px] flex-row items-center gap-2 rounded-[12px] px-2 active:opacity-80 ${props.selected ? "bg-card border border-border" : ""}`}
      style={{ paddingLeft: 8 + props.depth * 18 }}
    >
      {isDir ? (
        props.expanded ? (
          <ChevronDown size={12} color={theme.colors.text.muted} />
        ) : (
          <ChevronRight size={12} color={theme.colors.text.muted} />
        )
      ) : (
        <View className="w-3" />
      )}

      {isDir ? (
        props.expanded ? (
          <FolderOpen size={17} color={theme.colors.text.secondary} />
        ) : (
          <Folder size={17} color={theme.colors.text.secondary} />
        )
      ) : (
        <FileIcon filename={props.entry.name} size={17} />
      )}

      <Text
        className={`min-w-0 flex-1 text-sm leading-normal ${props.selected ? "font-semibold text-foreground" : "font-medium text-foreground-secondary"}`}
        numberOfLines={1}
      >
        {props.entry.name}
      </Text>
    </Pressable>
  );
});
