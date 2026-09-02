import React, { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator } from "react-native";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { HelpCircle, Check } from "lucide-react-native";
import type { AskQuestionRequest } from "@console/types";
import { theme } from "@/styles/theme";

interface QuestionPanelProps {
  request: AskQuestionRequest;
  /** Total questions in the batch (for the "Question X of Y" indicator). */
  total: number;
  /** 1-based index of the current question in the batch. */
  index: number;
  /** True when this is the last question — the primary button becomes "Submit all". */
  isLast: boolean;
  submitting: boolean;
  /** Called with the user's answer to this question (selected options or typed text). */
  onAnswer: (answer: string | string[]) => void;
  /** Called when the user chooses to skip this question. */
  onSkip: () => void;
}

export function QuestionPanel({
  request,
  total,
  index,
  isLast,
  submitting,
  onAnswer,
  onSkip,
}: QuestionPanelProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const paddingBottom = keyboardVisible ? 8 : Math.max(insets.bottom, 8) + 4;

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [customAnswer, setCustomAnswer] = useState("");

  const hasOptions = request.options != null && request.options.length > 0;
  const hasAnswer = customAnswer.trim().length > 0 || selected.size > 0;

  const toggle = (option: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (request.isMultiSelect) {
        if (next.has(option)) next.delete(option);
        else next.add(option);
      } else {
        next.clear();
        next.add(option);
      }
      return next;
    });
  };

  const getAnswer = (): string | string[] | null => {
    if (customAnswer.trim()) return customAnswer.trim();
    if (selected.size === 0) return null;
    return request.isMultiSelect ? [...selected] : [...selected][0]!;
  };

  const handlePrimary = () => {
    const answer = getAnswer();
    if (answer !== null) onAnswer(answer);
  };

  return (
    <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
      <View className="px-3 pt-2 bg-screen" style={{ paddingBottom }}>
        <View className="rounded-2xl border border-white/15 bg-[#18181b] p-4 gap-3">
          {/* Header */}
          <View className="flex-row items-center gap-2.5">
            <HelpCircle size={18} color="#ffffff" />
            <Text className="text-sm font-semibold text-white flex-1">
              {request.question}
            </Text>
            {total > 1 && (
              <Text className="text-[11px] font-mono text-foreground-secondary">
                {index} of {total}
              </Text>
            )}
          </View>

          {/* Multiple-choice Options */}
          {hasOptions && (
            <View className="gap-2 pl-6">
              {request.options!.map((option) => {
                const isSelected = selected.has(option);
                return (
                  <Pressable
                    key={option}
                    onPress={() => toggle(option)}
                    className={`w-full flex-row items-center gap-3 px-3.5 py-2.5 rounded-xl border ${
                      isSelected
                        ? "border-white/40 bg-white/[0.09]"
                        : "border-white/10 bg-white/[0.02]"
                    }`}
                    style={({ pressed }) => ({
                      opacity: pressed ? 0.75 : 1,
                    })}
                  >
                    {/* Checkbox / Radio indicator */}
                    <View
                      className={`w-4 h-4 items-center justify-center ${
                        request.isMultiSelect ? "rounded" : "rounded-full"
                      } ${
                        isSelected
                          ? "bg-white"
                          : "border border-white/40 bg-transparent"
                      }`}
                    >
                      {isSelected ? (
                        request.isMultiSelect ? (
                          <Check size={11} color="#000000" strokeWidth={3.5} />
                        ) : (
                          <View className="w-1.5 h-1.5 rounded-full bg-black" />
                        )
                      ) : null}
                    </View>

                    <Text
                      className={`text-sm flex-1 ${
                        isSelected ? "text-white font-medium" : "text-foreground-secondary"
                      }`}
                    >
                      {option}
                    </Text>
                  </Pressable>
                );
              })}
            </View>
          )}

          {/* Custom Text Input */}
          <View className="pl-6">
            <TextInput
              value={customAnswer}
              onChangeText={setCustomAnswer}
              onSubmitEditing={() => {
                if (hasAnswer) handlePrimary();
              }}
              placeholder={hasOptions ? "Or type your own answer…" : "Type your answer…"}
              placeholderTextColor={theme.colors.text.muted}
              className="w-full px-3.5 py-2.5 rounded-xl border border-white/10 bg-black/40 text-sm text-white"
              returnKeyType="send"
            />
          </View>

          {/* Action Buttons */}
          <View className="flex-row items-center justify-between pl-6 pt-1">
            <View>
              {request.skippable !== false && (
                <Pressable
                  onPress={onSkip}
                  disabled={submitting}
                  className="px-4 py-2 rounded-full border border-white/15 bg-transparent"
                  style={({ pressed }) => ({
                    opacity: pressed || submitting ? 0.5 : 1,
                  })}
                >
                  <Text className="text-xs font-semibold text-foreground-secondary">
                    Skip
                  </Text>
                </Pressable>
              )}
            </View>

            <Pressable
              onPress={handlePrimary}
              disabled={!hasAnswer || submitting}
              className={`px-5 py-2.5 rounded-full ${
                hasAnswer && !submitting
                  ? "bg-white"
                  : "bg-white/10 opacity-40"
              }`}
              style={({ pressed }) => ({
                opacity: pressed && hasAnswer && !submitting ? 0.8 : undefined,
              })}
            >
              {submitting ? (
                <ActivityIndicator size="small" color="#000000" />
              ) : (
                <Text
                  className={`text-xs font-bold ${
                    hasAnswer && !submitting ? "text-black" : "text-white/60"
                  }`}
                >
                  {isLast ? (total > 1 ? "Submit all" : "Submit") : "Next"}
                </Text>
              )}
            </Pressable>
          </View>
        </View>
      </View>
    </KeyboardStickyView>
  );
}
