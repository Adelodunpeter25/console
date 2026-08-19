import React, { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator } from "react-native";
import { HelpCircle } from "lucide-react-native";
import type { AskQuestionRequest } from "@console/types";
import { theme } from "../../styles/theme";

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
    <View className="mx-4 mb-2 rounded-2xl border border-white/15 bg-card p-4 gap-3">
      {/* Header */}
      <View className="flex-row items-center gap-2.5">
        <HelpCircle size={17} color={theme.colors.accent} />
        <Text className="text-sm font-semibold text-foreground flex-1">
          {request.question}
        </Text>
        {total > 1 && (
          <Text className="text-[11px] font-mono text-foreground-secondary">
            {index} of {total}
          </Text>
        )}
      </View>

      {/* Options */}
      {hasOptions && (
        <View className="gap-2 pl-6">
          {request.options!.map((option) => {
            const isSelected = selected.has(option);
            return (
              <Pressable
                key={option}
                onPress={() => toggle(option)}
                className={`w-full flex-row items-center gap-2.5 px-3 py-2.5 rounded-xl border ${
                  isSelected
                    ? "border-accent bg-accent/15"
                    : "border-white/10 bg-white/[0.02]"
                }`}
                style={({ pressed }) => ({
                  opacity: pressed ? 0.75 : 1,
                })}
              >
                <View
                  className={`w-4 h-4 items-center justify-center border-2 ${
                    request.isMultiSelect ? "rounded-sm" : "rounded-full"
                  } ${isSelected ? "border-accent bg-accent/20" : "border-foreground-secondary/50"}`}
                >
                  {isSelected && (
                    <View className="w-1.5 h-1.5 rounded-full bg-accent" />
                  )}
                </View>
                <Text
                  className={`text-sm flex-1 ${
                    isSelected
                      ? "text-foreground font-medium"
                      : "text-foreground-secondary"
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
          placeholderTextColor="#71717a"
          className="w-full px-3.5 py-2.5 rounded-xl border border-white/10 bg-black/40 text-sm text-foreground"
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
              className="px-4 py-2 rounded-xl border border-white/10 bg-white/[0.02]"
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
          className={`px-5 py-2 rounded-xl border ${
            hasAnswer && !submitting
              ? "border-accent/40 bg-accent/20"
              : "border-white/10 bg-white/[0.04] opacity-40"
          }`}
          style={({ pressed }) => ({
            opacity: pressed && hasAnswer && !submitting ? 0.75 : undefined,
          })}
        >
          {submitting ? (
            <ActivityIndicator size="small" color={theme.colors.accent} />
          ) : (
            <Text className="text-xs font-bold text-accent">
              {isLast ? (total > 1 ? "Submit all" : "Submit") : "Next"}
            </Text>
          )}
        </Pressable>
      </View>
    </View>
  );
}
