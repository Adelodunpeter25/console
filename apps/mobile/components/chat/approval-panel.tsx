import React, { useState } from "react";
import { View, Text, TouchableOpacity, ActivityIndicator } from "react-native";
import { Check, ShieldAlert, X } from "lucide-react-native";
import type { PendingPermission, PendingQuestion } from "../../types";
import { theme } from "../../styles/theme";

interface ApprovalPanelProps {
  pendingPermission: PendingPermission | null;
  pendingQuestion: PendingQuestion | null;
  onApprove: (allow: boolean) => Promise<void>;
  onAnswer: (answer: string | string[]) => Promise<void>;
}

/**
 * Renders the pending permission (Allow/Deny) or pending question (options)
 * that the agent is waiting on. The parent clears the pending state after the
 * decision is posted.
 */
export function ApprovalPanel({
  pendingPermission,
  pendingQuestion,
  onApprove,
  onAnswer,
}: ApprovalPanelProps) {
  const [submitting, setSubmitting] = useState(false);

  if (!pendingPermission && !pendingQuestion) return null;

  const isPermission = Boolean(pendingPermission);

  const handleApprove = async (allow: boolean) => {
    setSubmitting(true);
    try {
      await onApprove(allow);
    } finally {
      setSubmitting(false);
    }
  };

  const handleAnswer = async (answer: string | string[]) => {
    setSubmitting(true);
    try {
      await onAnswer(answer);
    } finally {
      setSubmitting(false);
    }
  };

  const args = isPermission
    ? JSON.stringify(pendingPermission?.request.args ?? {}, null, 2).slice(0, 600)
    : "";

  return (
    <View className="mx-4 mb-2 rounded-2xl p-4 gap-3 border border-border bg-card">
      <View className="flex-row items-center gap-2.5">
        <View
          className="w-8 h-8 rounded-full items-center justify-center"
          style={{ backgroundColor: theme.colors.status.attentionBg }}
        >
          <ShieldAlert size={16} color={theme.colors.status.attention} />
        </View>
        <Text className="text-sm font-bold text-foreground flex-1">
          {isPermission ? "Permission required" : "Question from agent"}
        </Text>
      </View>

      <View className="gap-1.5">
        <Text className="text-sm font-semibold text-foreground">
          {isPermission
            ? `Run ${pendingPermission?.request.toolName ?? "tool"}?`
            : pendingQuestion?.request.question}
        </Text>
        {isPermission && pendingPermission?.request.reason ? (
          <Text className="text-xs text-foreground-secondary leading-5">
            {String(pendingPermission.request.reason)}
          </Text>
        ) : null}
        {args ? (
          <View
            className="mt-1 rounded-xl px-3 py-2.5"
            style={{ backgroundColor: "rgba(255,255,255,0.05)" }}
          >
            <Text className="text-[11px] font-mono text-foreground-secondary leading-4" numberOfLines={5}>
              {args}
            </Text>
          </View>
        ) : null}
      </View>

      {isPermission ? (
        <View className="flex-row gap-2.5">
          <TouchableOpacity
            className="flex-1 py-2.5 rounded-full items-center justify-center flex-row gap-2"
            style={{ backgroundColor: theme.colors.text.primary }}
            onPress={() => handleApprove(true)}
            disabled={submitting}
            activeOpacity={0.8}
          >
            {submitting ? (
              <ActivityIndicator size="small" color={theme.colors.text.dark} />
            ) : (
              <>
                <Check size={16} color={theme.colors.text.dark} />
                <Text className="text-sm font-bold text-black">Allow</Text>
              </>
            )}
          </TouchableOpacity>
          <TouchableOpacity
            className="flex-1 py-2.5 rounded-full border items-center justify-center flex-row gap-2"
            style={{ borderColor: theme.colors.border }}
            onPress={() => handleApprove(false)}
            disabled={submitting}
            activeOpacity={0.8}
          >
            <X size={15} color={theme.colors.text.primary} />
            <Text className="text-sm font-bold text-foreground">Deny</Text>
          </TouchableOpacity>
        </View>
      ) : (
        <View className="gap-2">
          {(pendingQuestion?.request.options ?? []).map((option: string, idx: number) => (
            <TouchableOpacity
              key={idx}
              className="py-2.5 px-4 rounded-full border items-center justify-center"
              style={{ borderColor: theme.colors.border, backgroundColor: "rgba(255,255,255,0.04)" }}
              onPress={() =>
                handleAnswer(pendingQuestion?.request.isMultiSelect ? [option] : option)
              }
              disabled={submitting}
              activeOpacity={0.8}
            >
              <Text className="text-sm font-semibold text-foreground">{option}</Text>
            </TouchableOpacity>
          ))}
        </View>
      )}
    </View>
  );
}