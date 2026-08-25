import React, { useRef, useCallback, useState } from "react";
import { View, Text, Pressable } from "react-native";
import { BottomSheetModal, BottomSheetScrollView } from "@gorhom/bottom-sheet";
import { Server, Check, Plus, ChevronLeft } from "lucide-react-native";
import { SharedBottomSheet } from "@/components/common/shared-bottom-sheet";
import { EnvironmentEditor } from "@/components/environments/environment-editor";
import { activateEnvironment, environments$, probeEnvironment } from "@/stores/useEnvironmentsStore";
import { useValue } from "@legendapp/state/react";
import { urlHost } from "@/utils/url";

function StatusDot({ ok, checkedAt }: { ok?: boolean; checkedAt?: number }) {
  const color = ok === undefined ? "#52525b" : ok ? "#34d399" : "#f87171";
  return <View className="w-2 h-2 rounded-full mr-3" style={{ backgroundColor: color }} />;
}

/**
 * Home-header environment switcher: a server-icon trigger that opens a
 * bottom sheet listing environments with their probe status. Tapping another
 * environment activates it and clears server-scoped caches. The "+ Add
 * environment" row opens the EnvironmentEditor inline inside the sheet — no
 * navigation required.
 */
export function EnvironmentSwitcher() {
  const sheetRef = useRef<BottomSheetModal>(null);
  const environments = useValue(environments$.environments);
  const activeId = useValue(environments$.activeId);
  const probes = useValue(environments$.probes);

  // When true the sheet shows the create form instead of the list.
  const [creating, setCreating] = useState(false);

  const open = useCallback(() => {
    setCreating(false);
    sheetRef.current?.present();
    // Probe all environments so the dots are fresh while the sheet is open.
    environments$.environments.peek().forEach((env) => {
      void probeEnvironment(env.id);
    });
  }, []);

  const handleSelect = (envId: string) => {
    if (envId === activeId) {
      sheetRef.current?.dismiss();
      return;
    }
    activateEnvironment(envId);
    sheetRef.current?.dismiss();
  };

  return (
    <>
      <Pressable
        className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center mr-2"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={open}
      >
        <Server size={18} color="#ffffff" />
      </Pressable>

      <SharedBottomSheet ref={sheetRef} snapPoints={["50%", "80%"]}>
        {creating ? (
          /* ── Create form ── */
          <View style={{ flex: 1 }}>
            {/* Mini header inside sheet */}
            <View className="flex-row items-center mb-4">
              <Pressable
                onPress={() => setCreating(false)}
                style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                className="w-8 h-8 rounded-full bg-card border border-border items-center justify-center mr-3"
              >
                <ChevronLeft size={16} color="#ffffff" />
              </Pressable>
              <Text className="text-base font-semibold text-foreground">Add environment</Text>
            </View>
            <BottomSheetScrollView
              keyboardShouldPersistTaps="handled"
              showsVerticalScrollIndicator={false}
              contentContainerStyle={{ paddingBottom: 32 }}
            >
              <EnvironmentEditor
                key="switcher-create"
                mode="create"
                insideBottomSheet
                onDone={() => {
                  setCreating(false);
                  sheetRef.current?.dismiss();
                }}
              />
            </BottomSheetScrollView>
          </View>
        ) : (
          /* ── Environment list ── */
          <View style={{ flex: 1 }}>
            <Text className="text-base font-semibold text-foreground mb-3">Environments</Text>
            {environments.map((env) => {
              const probe = probes[env.id];
              const isActive = env.id === activeId;
              return (
                <Pressable
                  key={env.id}
                  onPress={() => handleSelect(env.id)}
                  style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                  className={`flex-row items-center rounded-2xl border px-4 py-3.5 mb-2 ${
                    isActive ? "bg-card-alt border-border" : "bg-card border-border/50"
                  }`}
                >
                  <StatusDot ok={probe?.ok} checkedAt={probe?.checkedAt} />
                  <View className="flex-1">
                    <Text className="text-sm font-semibold text-foreground">{env.name}</Text>
                    <Text className="text-xs text-foreground-secondary mt-0.5" numberOfLines={1}>
                      {urlHost(env.url)}
                    </Text>
                  </View>
                  {isActive ? <Check size={16} color="#34d399" /> : null}
                </Pressable>
              );
            })}

            <Pressable
              onPress={() => setCreating(true)}
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              className="flex-row items-center justify-center gap-2 rounded-2xl border border-dashed border-border py-3.5 mt-1"
            >
              <Plus size={16} color="#a1a1aa" />
              <Text className="text-sm font-medium text-foreground-secondary">
                Add environment
              </Text>
            </Pressable>
          </View>
        )}
      </SharedBottomSheet>
    </>
  );
}
