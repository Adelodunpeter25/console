import React, { useRef, useEffect } from "react";
import { View, Text, Pressable, ScrollView, ActivityIndicator } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { ShieldCheck, Check } from "lucide-react-native";
import type { ApprovalMode } from "@console/types";
import { SharedBottomSheet } from "../../common/shared-bottom-sheet";
import { useProviderStore } from "../../../stores";
import { theme } from "../../../styles/theme";

interface ApprovalModePickerSheetProps {
  value: ApprovalMode;
  onChange: (mode: ApprovalMode) => void;
}

export function ApprovalModePickerSheet({ value, onChange }: ApprovalModePickerSheetProps) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);
  const approvalModes = useProviderStore((state) => state.approvalModes);
  const loadingApprovalModes = useProviderStore((state) => state.loadingApprovalModes);
  const loadApprovalModes = useProviderStore((state) => state.loadApprovalModes);

  useEffect(() => {
    void loadApprovalModes();
  }, [loadApprovalModes]);

  const activeModeObj = approvalModes.find((m) => m.value === value);
  const currentLabel = activeModeObj?.label ?? value;

  return (
    <>
      <Pressable
        className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-lg bg-card-alt/70 border border-border/50 shrink-0"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={() => {
          if (approvalModes.length === 0) void loadApprovalModes();
          bottomSheetRef.current?.present();
        }}
      >
        <ShieldCheck size={13} color={theme.colors.text.secondary} />
        <Text className="text-xs font-medium text-foreground">{currentLabel}</Text>
      </Pressable>

      <SharedBottomSheet ref={bottomSheetRef} title="Approval Mode" snapPoints={["50%"]}>
        <ScrollView showsVerticalScrollIndicator={false}>
          {loadingApprovalModes && approvalModes.length === 0 ? (
            <View className="items-center justify-center py-10">
              <ActivityIndicator size="small" color={theme.colors.text.muted} />
            </View>
          ) : (
            approvalModes.map((item) => {
              const isSelected = item.value === value;
              return (
                <Pressable
                  key={item.value}
                  className={`flex-row items-center justify-between px-3.5 py-3 rounded-xl mb-1.5 ${
                    isSelected ? "bg-card-alt border border-border/60" : ""
                  }`}
                  style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                  onPress={() => {
                    onChange(item.value as ApprovalMode);
                    bottomSheetRef.current?.dismiss();
                  }}
                >
                  <View className="flex-1 mr-2">
                    <Text className="text-sm font-semibold text-foreground mb-0.5">{item.label}</Text>
                    <Text className="text-xs text-foreground-secondary">{item.description}</Text>
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
