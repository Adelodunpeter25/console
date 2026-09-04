import React, { useCallback, useEffect, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, BackHandler, ScrollView } from "react-native";
import { LegendList } from "@legendapp/list/react-native";
import { RefreshCw } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { DiffView } from "@/components/chat/tools/diff-view";
import { ChangesRowItem } from "@/components/changes/ChangesRows";
import { useChanges } from "@/hooks/useChanges";
import { app$, setActiveTab } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { statusColor, statusLetter, type ChangesScope } from "@/utils/changes";
import { theme } from "@/styles/theme";

const SCOPES: ChangesScope[] = ["turn", "all"];

export function ChangesScreen() {
  const previousTab = useValue(app$.previousTab);
  const vm = useChanges();
  const goBack = useCallback(() => {
    setActiveTab(previousTab && previousTab !== "changes" ? previousTab : "chat");
  }, [previousTab]);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      if (vm.selectedPath) { vm.clearSelection(); return true; }
      goBack();
      return true;
    });
    return () => sub.remove();
  }, [vm.selectedPath, vm.clearSelection, goBack]);

  const renderItem = useCallback(({ item }: { item: (typeof vm.rows)[number] }) => (
    <ChangesRowItem
      item={item}
      collapsed={item.kind === "folder" ? vm.collapsed.has(item.name) : false}
      onToggleFolder={vm.toggleFolder}
      onSelectFile={vm.selectFile}
    />
  ), [vm.collapsed, vm.toggleFolder, vm.selectFile]);

  const fallbackAdded = useMemo(() => {
    if (vm.diff || !vm.selectedChange) return null;
    if (vm.selectedChange.status === "added") return vm.selectedChange.additions ?? 0;
    return null;
  }, [vm.diff, vm.selectedChange]);

  if (vm.selectedPath) {
    return (
      <View className="flex-1 bg-screen">
        <ScreenHeader title={vm.selectedName} subtitle={vm.selectedRel} onBack={vm.clearSelection} />
        <ScrollView className="flex-1" contentContainerStyle={{ padding: 12, paddingBottom: 40 }}>
          {vm.selectedChange ? (
            <View className="flex-row items-center gap-2 mb-3">
              <Text style={{ color: statusColor(vm.selectedChange.status) }} className="text-xs font-bold">{statusLetter(vm.selectedChange.status)}</Text>
              <Text className="text-xs font-mono text-emerald-400">+{vm.selectedChange.additions ?? 0}</Text>
              <Text className="text-xs font-mono text-red-400">-{vm.selectedChange.deletions ?? 0}</Text>
            </View>
          ) : null}
          {vm.diffLoading ? (
            <View className="items-center py-10"><ActivityIndicator color={theme.colors.text.muted} /></View>
          ) : vm.diff ? (
            <DiffView diff={vm.diff} filePath={vm.selectedPath} />
          ) : fallbackAdded != null ? (
            <View className="rounded-xl bg-black/40 p-3"><Text className="text-xs font-mono text-foreground-secondary">New file +{fallbackAdded} lines (diff unavailable off-git).</Text></View>
          ) : (
            <View className="rounded-xl bg-black/40 p-3"><Text className="text-xs font-mono text-foreground-secondary">No diff available for this file.</Text></View>
          )}
        </ScrollView>
      </View>
    );
  }

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title={vm.branch ?? "Changes"}
        subtitle={`${vm.totals.files} files  +${vm.totals.additions} -${vm.totals.deletions}`}
        onBack={goBack}
        rightAction={
          <Pressable onPress={vm.refresh} className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center" style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}>
            <RefreshCw size={16} color={theme.colors.text.secondary} />
          </Pressable>
        }
      />
      <View className="flex-row px-4 pb-2 gap-2">
        {SCOPES.map((s) => (
          <Pressable key={s} onPress={() => vm.setScope(s)} className={`px-3 py-1.5 rounded-full border ${vm.scope === s ? "bg-foreground border-foreground" : "bg-card border-border"}`}>
            <Text className={`text-xs font-semibold ${vm.scope === s ? "text-black" : "text-foreground-secondary"}`}>{s === "turn" ? "This Turn" : "All Turns"}</Text>
          </Pressable>
        ))}
      </View>
      {vm.isLoading ? (
        <View className="flex-1 items-center justify-center"><ActivityIndicator color={theme.colors.text.muted} /><Text className="mt-2 text-xs text-foreground-muted">Loading changes…</Text></View>
      ) : vm.error ? (
        <View className="flex-1 items-center justify-center px-6"><Text className="text-sm font-bold text-foreground">Couldn&apos;t load changes</Text><Text className="mt-1 text-xs text-foreground-muted">{(vm.error as Error).message}</Text></View>
      ) : vm.rows.length === 0 ? (
        <View className="flex-1 items-center justify-center px-6"><Text className="text-sm font-bold text-foreground">No working tree changes</Text><Text className="mt-1 text-xs text-foreground-muted text-center">Edits from this session will appear here.</Text></View>
      ) : (
        <LegendList
          data={vm.rows}
          keyExtractor={(item) => item.key}
          recycleItems={false}
          estimatedItemSize={44}
          extraData={vm.rowsFingerprint}
          contentContainerStyle={{ paddingBottom: 24 }}
          renderItem={renderItem}
        />
      )}
      {vm.isFetching ? <View className="absolute top-1 right-4"><ActivityIndicator size="small" color={theme.colors.text.muted} /></View> : null}
    </View>
  );
}

export default ChangesScreen;
