import React, { useRef } from "react";
import { View, Text, Pressable, ScrollView } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { ShieldCheck, Check } from "lucide-react-native";
import type { ApprovalMode } from "@console/types";
import { SharedBottomSheet } from "../../common/shared-bottom-sheet";
import { theme } from "../../../styles/theme";

interface ApprovalModePickerSheetProps {
  value: ApprovalMode;
  onChange: (mode: ApprovalMode) => void;
}

const MODES: Array<{ mode: ApprovalMode; label: string; description: string }> = [
  {
    mode: "always-ask",
    label: "Always ask",
    description: "Review and approve all agent actions and commands before execution.",
  },
  {
    mode: "auto",
    label: "Full auto",
    description: "Let the agent execute commands and edits automatically without prompts.",
  },
];

export function ApprovalModePickerSheet({ value, onChange }: ApprovalModePickerSheetProps) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);
  const currentMode = MODES.find((m) => m.mode === value) ?? MODES[0];

  return (
    <>
      <Pressable
        className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-lg bg-card-alt/70 border border-border/50"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={() => bottomSheetRef.current?.present()}
      >
        <ShieldCheck size={13} color={theme.colors.text.secondary} />
        <Text className="text-xs font-medium text-foreground">{currentMode.label}</Text>
      </Pressable>

      <SharedBottomSheet ref={bottomSheetRef} title="Approval Mode" snapPoints={["35%"]}>
        <ScrollView showsVerticalScrollIndicator={false}>
          {MODES.map((item) => {
            const isSelected = item.mode === value;
            return (
              <Pressable
                key={item.mode}
                className={`flex-row items-center justify-between px-3.5 py-3 rounded-xl mb-1.5 ${
                  isSelected ? "bg-card-alt border border-border/60" : ""
                }`}
                style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                onPress={() => {
                  onChange(item.mode);
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
          })}
        </ScrollView>
      </SharedBottomSheet>
    </>
  );
}
