import React from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import { ArrowUp, Square } from "lucide-react-native";
import { theme } from "../../styles/theme";
import { useAppStore, useProjectStore, useSessionStore } from "../../stores";

import { ComposerBottomStrip } from "./composer-bottom-strip";

interface ComposerProps {
  value: string;
  onChangeText: (text: string) => void;
  onSend: () => void;
  onStop?: () => void;
  running?: boolean;
}

/** Chat input row matching unified pill with bottom selectors strip. */
export function Composer({ value, onChangeText, onSend, onStop, running }: ComposerProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const canSend = value.trim().length > 0;
  const paddingBottom = keyboardVisible ? 8 : Math.max(insets.bottom, 8) + 4;

  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const projects = useProjectStore((state) => state.projects);
  const sessionView = useSessionStore((state) =>
    selectedSessionId ? state.getSession(selectedSessionId) : undefined,
  );
  const changeModel = useSessionStore((state) => state.changeModel);
  const changeProject = useSessionStore((state) => state.changeProject);
  const setApprovalMode = useSessionStore((state) => state.setApprovalMode);

  const selectedProject = projects.find(
    (p) => p.path === sessionView?.sessionCwd || (sessionView?.sessionCwd && p.path.endsWith(sessionView.sessionCwd)),
  );

  return (
    <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
      <View
        className="px-4 pt-2 bg-screen border-t border-border"
        style={{ paddingBottom }}
      >
        <View className="flex-row items-center bg-card border border-border/80 rounded-full pl-4 pr-1.5 py-1 min-h-[48px]">
          <TextInput
            style={styles.input}
            value={value}
            onChangeText={onChangeText}
            placeholder="Ask the repo agent, or run a command…"
            placeholderTextColor={theme.colors.text.muted}
            multiline
            textAlignVertical="center"
          />
          {running && onStop ? (
            <Pressable
              className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
              style={({ pressed }) => ({
                backgroundColor: "rgba(255,255,255,0.12)",
                opacity: pressed ? 0.7 : 1,
              })}
              onPress={onStop}
            >
              <Square size={13} color={theme.colors.text.primary} />
            </Pressable>
          ) : (
            <Pressable
              className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
              style={({ pressed }) => ({
                backgroundColor: canSend ? theme.colors.text.primary : "rgba(255,255,255,0.08)",
                opacity: pressed && canSend ? 0.8 : 1,
              })}
              onPress={onSend}
              disabled={!canSend}
            >
              <ArrowUp size={16} color={canSend ? theme.colors.text.dark : theme.colors.text.muted} />
            </Pressable>
          )}
        </View>

        {/* Bottom Selector Strip: Project, Model, Approval Mode */}
        {selectedSessionId ? (
          <ComposerBottomStrip
            projects={projects}
            selectedProjectId={selectedProject?.id ?? null}
            onProjectChange={(project) => changeProject(selectedSessionId, project)}
            selectedModel={sessionView?.sessionModelId ?? null}
            selectedProvider={sessionView?.sessionProvider ?? null}
            onModelChange={(modelId) => changeModel(selectedSessionId, modelId)}
            approvalMode={sessionView?.approvalMode ?? "always-ask"}
            onApprovalModeChange={(mode) => setApprovalMode(selectedSessionId, mode)}
          />
        ) : null}
      </View>
    </KeyboardStickyView>
  );
}

const styles = StyleSheet.create({
  input: {
    flex: 1,
    maxHeight: 120,
    paddingTop: 8,
    paddingBottom: 8,
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 19,
    includeFontPadding: false,
  },
});
