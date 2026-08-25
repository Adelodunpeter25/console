import React, { useState } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { ChevronRight, Plus, Unplug, X } from "lucide-react-native";
import { ScreenHeader } from "@/components/layout/screen-header";
import { GlassSurface } from "@/components/layout/glass-surface";
import { EnvironmentEditor } from "@/components/environments/environment-editor";
import {
  deactivate as deactivateEnvironment,
  environments$,
  type Environment,
} from "@/stores/useEnvironmentsStore";
import { useValue } from "@legendapp/state/react";
import { urlHost } from "@/utils/url";
import { theme } from "@/styles/theme";

/**
 * Settings → Connection: the environment list replacing the old single-URL
 * endpoint editor. Tapping a row (or "+ Add" in the header) opens the shared
 * EnvironmentEditor as a full screen with back/cancel and save/delete.
 */
export function EnvironmentsSettings({ onBack }: { onBack?: () => void }) {
  const environments = useValue(environments$.environments);
  const activeId = useValue(environments$.activeId);
  const probes = useValue(environments$.probes);

  // undefined = list view; null = create; string = edit that env.
  const [editing, setEditing] = useState<string | null | undefined>(undefined);

  if (editing !== undefined) {
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.background }}>
        <ScreenHeader
          title={editing ? "Edit environment" : "Add environment"}
          onBack={() => setEditing(undefined)}
          rightAction={
            <Pressable
              className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              onPress={() => setEditing(undefined)}
            >
              <X size={18} color="#ffffff" />
            </Pressable>
          }
        />
        <ScrollView
          className="flex-1"
          contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
          keyboardShouldPersistTaps="handled"
        >
          <EnvironmentEditor
            key={editing ?? "__create__"}
            mode={editing ? "edit" : "create"}
            envId={editing ?? undefined}
            onDone={() => setEditing(undefined)}
          />
        </ScrollView>
      </View>
    );
  }

  const renderRow = (env: Environment) => {
    const probe = probes[env.id];
    const isActive = env.id === activeId;
    return (
      <Pressable
        key={env.id}
        onPress={() => setEditing(env.id)}
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        className="flex-row items-center rounded-xl px-3 py-2 mb-0.5"
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
    <View style={{ flex: 1, backgroundColor: theme.colors.background }}>
      <ScreenHeader
        title="Connection"
        onBack={onBack}
        rightAction={
          <Pressable
            onPress={() => setEditing(null)}
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            className="flex-row items-center gap-1.5 rounded-full bg-card border border-border px-4 py-2"
          >
            <Plus size={14} color="#ffffff" />
            <Text className="text-xs font-semibold text-foreground">Add</Text>
          </Pressable>
        }
      />
      <ScrollView
        style={{ flex: 1 }}
        contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
      >
        <GlassSurface className="mb-4 p-4">
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
      </ScrollView>
    </View>
  );
}
