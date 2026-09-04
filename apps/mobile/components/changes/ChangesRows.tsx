import React, { memo } from "react";
import { View, Text, Pressable } from "react-native";
import { ChevronDown, ChevronRight } from "lucide-react-native";
import { FileIcon } from "@/components/icons/file-icon";
import { statusColor, statusLetter, type ChangesRow } from "@/utils/changes";
import { theme } from "@/styles/theme";

export const FolderRow = memo(function FolderRow({
  name,
  count,
  additions,
  deletions,
  collapsed,
  onToggle,
}: {
  name: string;
  count: number;
  additions: number;
  deletions: number;
  collapsed: boolean;
  onToggle: (name: string) => void;
}) {
  return (
    <Pressable onPress={() => onToggle(name)} className="flex-row items-center px-4 py-2 gap-1.5 active:opacity-70">
      {collapsed ? (
        <ChevronRight size={14} color={theme.colors.text.muted} />
      ) : (
        <ChevronDown size={14} color={theme.colors.text.muted} />
      )}
      <Text className="text-xs font-semibold text-foreground-secondary flex-1" numberOfLines={1}>
        {name} · {count}
      </Text>
      <Text className="text-[11px] font-mono text-emerald-400">+{additions}</Text>
      <Text className="text-[11px] font-mono text-red-400">-{deletions}</Text>
    </Pressable>
  );
});

export const FileRow = memo(function FileRow({
  name,
  rel,
  status,
  additions,
  deletions,
  onPress,
}: {
  name: string;
  rel: string;
  status: string;
  additions: number;
  deletions: number;
  onPress: () => void;
}) {
  return (
    <Pressable onPress={onPress} className="flex-row items-center px-4 py-2 gap-2 active:opacity-70">
      <Text style={{ color: statusColor(status) }} className="text-[11px] font-bold w-3 text-center">
        {statusLetter(status)}
      </Text>
      <FileIcon filename={name} size={16} />
      <View className="flex-1 min-w-0">
        <Text className="text-[13px] text-foreground" numberOfLines={1}>{name}</Text>
        <Text className="text-[11px] text-foreground-secondary" numberOfLines={1}>{rel}</Text>
      </View>
      <Text className="text-[11px] font-mono text-emerald-400">+{additions}</Text>
      <Text className="text-[11px] font-mono text-red-400">-{deletions}</Text>
      <ChevronRight size={14} color={theme.colors.text.muted} />
    </Pressable>
  );
});

export const ChangesRowItem = memo(function ChangesRowItem({
  item,
  collapsed,
  onToggleFolder,
  onSelectFile,
}: {
  item: ChangesRow;
  collapsed: boolean;
  onToggleFolder: (name: string) => void;
  onSelectFile: (path: string) => void;
}) {
  if (item.kind === "folder") {
    return (
      <FolderRow
        name={item.name}
        count={item.count}
        additions={item.additions}
        deletions={item.deletions}
        collapsed={collapsed}
        onToggle={onToggleFolder}
      />
    );
  }
  return (
    <FileRow
      name={item.name}
      rel={item.rel}
      status={item.status}
      additions={item.additions}
      deletions={item.deletions}
      onPress={() => onSelectFile(item.path)}
    />
  );
});
