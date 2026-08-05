import React, { useState } from "react";
import { View, Text, TouchableOpacity, ActivityIndicator } from "react-native";
import type { PendingPermission, PendingQuestion } from "../../types";

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
    ? JSON.stringify((pendingPermission?.request.args ?? {}), null, 2).slice(0, 400)
    : "";

  return (
    <View className="mx-4 mb-2 bg-card border border-border rounded-2xl p-4 gap-3">
      <Text className="text-[10px] font-mono font-bold text-foreground-secondary tracking-widest uppercase">
        {isPermission ? "Permission Required" : "Agent Question"}
      </Text>

      <View>
        <Text className="text-sm font-semibold text-foreground">
          {isPermission
            ? `Run ${(pendingPermission?.request.toolName ?? "tool")}?`
            : pendingQuestion?.request.question}
        </Text>
        {isPermission && pendingPermission?.request.reason ? (
          <Text className="text-xs text-foreground-secondary mt-1">
            {String(pendingPermission.request.reason)}
          </Text>
        ) : null}
        {args ? (
          <Text className="text-[10px] font-mono text-foreground-secondary leading-4 mt-1.5" numberOfLines={4}>
            {args}
          </Text>
        ) : null}
      </View>

      {isPermission ? (
        <View className="flex-row gap-2.5">
          <TouchableOpacity
            className="flex-1 py-2.5 rounded-full bg-foreground items-center justify-center"
            onPress={() => handleApprove(true)}
            disabled={submitting}
          >
            {submitting ? (
              <ActivityIndicator size="small" color="#000000" />
            ) : (
              <Text className="text-sm font-bold text-black">Allow</Text>
            )}
          </TouchableOpacity>
          <TouchableOpacity
            className="flex-1 py-2.5 rounded-full bg-transparent border border-border items-center justify-center"
            onPress={() => handleApprove(false)}
            disabled={submitting}
          >
            <Text className="text-sm font-bold text-foreground">Deny</Text>
          </TouchableOpacity>
        </View>
      ) : (
        <View className="gap-2">
          {(pendingQuestion?.request.options ?? []).map((option: string, idx: number) => (
            <TouchableOpacity
              key={idx}
              className="py-2.5 px-4 rounded-full bg-foreground/10 border border-border items-center justify-center"
              onPress={() =>
                handleAnswer(pendingQuestion?.request.isMultiSelect ? [option] : option)
              }
              disabled={submitting}
            >
              <Text className="text-sm font-semibold text-foreground">{option}</Text>
            </TouchableOpacity>
          ))}
        </View>
      )}
    </View>
  );
}
