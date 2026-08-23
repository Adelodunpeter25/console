import React, { useRef, useState } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { ChevronRight, Plus, Unplug } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { GlassSurface } from "@/components/layout/glass-surface";
import { SharedBottomSheet } from "@/components/common/shared-bottom-sheet";
import {
  useEnvironmentsStore,
  type Environment,
} from "@/stores/useEnvironmentsStore";
import { EnvironmentEditor } from "@/components/environments/environment-editor";
import { urlHost } from "@/utils/url";

/**
 * Settings → Connection: the environment list replacing the old single-URL
 * endpoint editor. Rows open the shared EnvironmentEditor component in a
 * bottom sheet, in edit or create mode.
 */
export function EnvironmentsSettings() {
  const environments = useEnvironmentsStore((state) => state.environments);
  const activeId = useEnvironmentsStore((state) => state.activeId);
  const probes = useEnvironmentsStore((state) => state.probes);
  const deactivate = useEnvironmentsStore((state) => state.deactivate);

  const sheetRef = useRef<BottomSheetModal>(null);
  // null = create mode.
  const [editingId, setEditingId] = useState<string | null>(null);

  const openEditor = (envId: string | null) => {
    setEditingId(envId);
    sheetRef.current?.present();
  };

  const renderRow = (env: Environment) => {
    const probe = probes[env.id];
    const isActive = env.id === activeId;
    return (
      <Pressable
        key={env.id}
        onPress={() => openEditor(env.id)}
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        className="flex-row items-center rounded-xl px-3 py-3 mb-1"
      >
        <View
          className="w-2 h-2 rounded-full mr-3"
          style={{ backgroundColor: probe?.ok === undefined ? "#52525b" : probe.ok ? "#34d399" : "#f87171" }}
        />
        <View className="flex-1">
          <View className="flex-row items-center gap-2">
            <Text className="text-sm font-semibold text-foreground">{env.name}</Text>
            {isActive ? (
              <View className="rounded-full bg-emerald-500/15 px-2 py-0.5">
                <Text className="text-[10px] font-semibold text-emerald-400">Active</Text>
              </View>
            ) : null}
          </View>
          <Text className="text-xs text-foreground-secondary mt-0.5" numberOfLines={1}>
            {urlHost(env.url)}
          </Text>
        </View>
        <ChevronRight size={18} color="#71717a" />
      </Pressable>
    );
  };

  return (
    <ScrollView
      style={{ flex: 1 }}
      contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
      keyboardShouldPersistTaps="handled"
      showsVerticalScrollIndicator={false}
    >
      <GlassSurface className="mb-4 p-4">
        <View className="flex-row items-center justify-between mb-2 px-1">
          <Text className="text-sm font-semibold text-foreground">Environments</Text>
          <Pressable
            onPress={() => openEditor(null)}
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            className="flex-row items-center gap-1.5 rounded-lg bg-card-alt/70 border border-border/50 px-3 py-1.5"
          >
            <Plus size={14} color="#ffffff" />
            <Text className="text-xs font-medium text-foreground">Add</Text>
          </Pressable>
        </View>
        {environments.length === 0 ? (
          <Text className="text-xs text-foreground-secondary px-1 py-3">
            No environments yet. Add a backend URL to get started.
          </Text>
        ) : (
          environments.map(renderRow)
        )}
      </GlassSurface>

      {activeId ? (
        <GlassSurface className="p-5 border-red-500/30 bg-red-500/5">
          <Pressable
            onPress={deactivate}
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            className="flex-row items-center justify-center gap-2"
          >
            <Unplug size={16} color="#f87171" />
            <Text className="text-sm font-medium text-red-400">Disconnect backend</Text>
          </Pressable>
        </GlassSurface>
      ) : null}

      <SharedBottomSheet
        ref={sheetRef}
        title={editingId ? "Edit environment" : "Add environment"}
        snapPoints={["70%", "85%"]}
      >
        {/* Keyed per target so form state resets between creates/edits. */}
        <EnvironmentEditor
          key={editingId ?? "__create__"}
          mode={editingId ? "edit" : "create"}
          envId={editingId ?? undefined}
          onDone={() => sheetRef.current?.dismiss()}
        />
      </SharedBottomSheet>
    </ScrollView>
  );
}

