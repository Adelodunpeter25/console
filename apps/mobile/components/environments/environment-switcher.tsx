import React, { useRef, useCallback } from "react";
import { View, Text, Pressable } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Server, Check, Plus } from "lucide-react-native";
import { SharedBottomSheet } from "@/components/common/shared-bottom-sheet";
import { confirmAlert } from "@/components/common/confirm-dialog";
import { useEnvironmentsStore } from "@/stores/useEnvironmentsStore";
import { urlHost } from "@/utils/url";

interface EnvironmentSwitcherProps {
  /** Opens the editor in create mode instead of embedding a "+" row. */
  onCreateEnvironment?: () => void;
}

function StatusDot({ ok, checkedAt }: { ok?: boolean; checkedAt?: number }) {
  const color = ok === undefined ? "#52525b" : ok ? "#34d399" : "#f87171";
  return <View className="w-2 h-2 rounded-full mr-3" style={{ backgroundColor: color }} />;
}

/**
 * Home-header environment switcher: a server-icon trigger that opens a
 * bottom sheet listing environments with their probe status. Tapping another
 * environment (after confirmation) activates it and clears server-scoped
 * caches.
 */
export function EnvironmentSwitcher({ onCreateEnvironment }: EnvironmentSwitcherProps) {
  const sheetRef = useRef<BottomSheetModal>(null);
  const environments = useEnvironmentsStore((state) => state.environments);
  const activeId = useEnvironmentsStore((state) => state.activeId);
  const probes = useEnvironmentsStore((state) => state.probes);

  const open = useCallback(() => {
    sheetRef.current?.present();
    // Probe all environments so the dots are fresh while the sheet is open.
    const store = useEnvironmentsStore.getState();
    store.environments.forEach((env) => {
      void store.probeEnvironment(env.id);
    });
  }, []);

  const handleSelect = (envId: string) => {
    if (envId === activeId) {
      sheetRef.current?.dismiss();
      return;
    }
    const env = environments.find((e) => e.id === envId);
    confirmAlert(
      "Switch environment?",
      `Connecting to "${env?.name ?? "this environment"}" clears cached chats for the current server.`,
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Switch",
          onPress: () => {
            useEnvironmentsStore.getState().activateEnvironment(envId);
            sheetRef.current?.dismiss();
          },
        },
      ],
    );
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

      <SharedBottomSheet ref={sheetRef} title="Environments" snapPoints={["45%", "70%"]}>
        <View className="flex-1">
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
            onPress={() => {
              sheetRef.current?.dismiss();
              onCreateEnvironment?.();
            }}
            style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
            className="flex-row items-center justify-center gap-2 rounded-2xl border border-dashed border-border py-3.5 mt-1"
          >
            <Plus size={16} color="#a1a1aa" />
            <Text className="text-sm font-medium text-foreground-secondary">
              Add environment
            </Text>
          </Pressable>
        </View>
      </SharedBottomSheet>
    </>
  );
}
