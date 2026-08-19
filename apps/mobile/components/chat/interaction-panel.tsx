import React, { useState } from "react";
import { View, Text, Pressable, ScrollView, ActivityIndicator } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { ShieldCheck, ShieldX } from "lucide-react-native";
import type { AskQuestionRequest, PermissionRequest } from "@console/types";
import { useChatStore } from "../../stores";
import { QuestionPanel } from "./question-panel";

/* ------------------------------------------------------------------ */
/* Permission request panel                                            */
/* ------------------------------------------------------------------ */

interface PermissionPanelProps {
  request: PermissionRequest;
  sessionId: string;
}

function PermissionPanel({ request, sessionId }: PermissionPanelProps) {
  const approvePermission = useChatStore((s) => s.approvePermission);
  const [submittingAction, setSubmittingAction] = useState<"allow" | "deny" | null>(null);

  const handleApprove = async (allow: boolean) => {
    setSubmittingAction(allow ? "allow" : "deny");
    try {
      await approvePermission(sessionId, request.requestId, allow);
    } finally {
      setSubmittingAction(null);
    }
  };

  const argsString = request.args != null ? JSON.stringify(request.args, null, 2) : "";
  const isBusy = submittingAction !== null;

  return (
    <View className="mx-4 mb-2 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 gap-3">
      {/* Header */}
      <View className="flex-row items-center gap-2.5">
        <ShieldCheck size={18} color="#f59e0b" />
        <Text className="text-sm font-medium text-foreground flex-1">
          {request.requiresUpgrade ? "Upgrade permission required: " : "Permission required: "}
          <Text className="font-mono text-amber-400 font-bold">{request.toolName}</Text>
        </Text>
      </View>

      {/* Reason */}
      {request.reason ? (
        <Text className="text-xs text-foreground-secondary pl-6 leading-5">
          {String(request.reason)}
        </Text>
      ) : null}

      {/* Arguments preview */}
      {argsString ? (
        <View className="pl-6">
          <View className="max-h-36 rounded-xl bg-black/40 p-2.5 overflow-hidden">
            <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
              <Text className="text-xs font-mono text-foreground-secondary leading-4" selectable>
                {argsString}
              </Text>
            </ScrollView>
          </View>
        </View>
      ) : null}

      {/* Actions */}
      <View className="flex-row items-center gap-2.5 pl-6 pt-1">
        <Pressable
          onPress={() => handleApprove(true)}
          disabled={isBusy}
          className="flex-row items-center gap-1.5 px-4 py-2 rounded-xl bg-emerald-500/15 border border-emerald-500/30"
          style={({ pressed }) => ({
            opacity: pressed || isBusy ? 0.6 : 1,
          })}
        >
          {submittingAction === "allow" ? (
            <ActivityIndicator size="small" color="#34d399" />
          ) : (
            <ShieldCheck size={14} color="#34d399" />
          )}
          <Text className="text-xs font-bold text-emerald-400">
            {request.requiresUpgrade ? "Allow once" : "Allow"}
          </Text>
        </Pressable>

        <Pressable
          onPress={() => handleApprove(false)}
          disabled={isBusy}
          className="flex-row items-center gap-1.5 px-4 py-2 rounded-xl bg-red-500/15 border border-red-500/30"
          style={({ pressed }) => ({
            opacity: pressed || isBusy ? 0.6 : 1,
          })}
        >
          {submittingAction === "deny" ? (
            <ActivityIndicator size="small" color="#f87171" />
          ) : (
            <ShieldX size={14} color="#f87171" />
          )}
          <Text className="text-xs font-bold text-red-400">Deny</Text>
        </Pressable>
      </View>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* Question wizard                                                     */
/* ------------------------------------------------------------------ */

function QuestionWizard({
  questions,
  sessionId,
}: {
  questions: AskQuestionRequest[];
  sessionId: string;
}) {
  const answerQuestion = useChatStore((s) => s.answerQuestion);
  const [index, setIndex] = useState(0);
  const [submitting, setSubmitting] = useState(false);

  const current = questions[index] ?? questions[0]!;
  const isLast = index >= questions.length - 1;

  const handleAnswerSubmit = async (requestId: string, answer: string | string[]) => {
    setSubmitting(true);
    try {
      await answerQuestion(sessionId, requestId, answer);
    } finally {
      setSubmitting(false);
      if (!isLast) {
        setIndex((i) => i + 1);
      }
    }
  };

  const handleSkip = async () => {
    setSubmitting(true);
    try {
      await answerQuestion(sessionId, current.requestId, "");
    } finally {
      setSubmitting(false);
      if (!isLast) {
        setIndex((i) => i + 1);
      }
    }
  };

  if (!current) return null;

  return (
    <QuestionPanel
      key={current.requestId}
      request={current}
      total={questions.length}
      index={Math.min(index + 1, questions.length)}
      isLast={isLast}
      submitting={submitting}
      onAnswer={(answer) => handleAnswerSubmit(current.requestId, answer)}
      onSkip={handleSkip}
    />
  );
}

/* ------------------------------------------------------------------ */
/* InteractionPanel Dispatcher                                         */
/* ------------------------------------------------------------------ */

interface InteractionPanelProps {
  sessionId?: string | null;
  pendingPermissions?: Array<{ request: PermissionRequest }>;
  pendingQuestions?: Array<{ request: AskQuestionRequest }>;
}

export function InteractionPanel({
  sessionId,
  pendingPermissions: propPermissions,
  pendingQuestions: propQuestions,
}: InteractionPanelProps) {
  const insets = useSafeAreaInsets();
  const currentSessionId = sessionId ?? "";
  const storePermissions = useChatStore(
    (s) => s.sessions[currentSessionId]?.pendingPermissions ?? [],
  );
  const storeQuestions = useChatStore(
    (s) => s.sessions[currentSessionId]?.pendingQuestions ?? [],
  );

  const permissions = propPermissions ?? storePermissions;
  const questions = propQuestions ?? storeQuestions;

  if (permissions.length === 0 && questions.length === 0) return null;

  if (permissions.length > 0) {
    return (
      <View style={{ paddingBottom: Math.max(insets.bottom, 8) + 4 }}>
        <PermissionPanel
          key={permissions[0]!.request.requestId}
          request={permissions[0]!.request}
          sessionId={currentSessionId}
        />
      </View>
    );
  }

  return (
    <QuestionWizard
      key={questions[0]!.request.batchId ?? questions[0]!.request.requestId}
      questions={questions.map((q) => q.request)}
      sessionId={currentSessionId}
    />
  );
}
