import React from "react";
import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { Trash2 } from "lucide-react-native";
import { KeyboardAvoidingView } from "react-native-keyboard-controller";
import { ScreenHeader } from "@/components/layout/screen-header";
import { ConsoleTerminalSurface } from "@/modules/console-terminal/src/terminal-surface";
import { ExtraKeysBar } from "@/components/terminal/extra-keys-bar";
import { ProjectPicker } from "@/components/terminal/project-picker";
import { RestartShellBar } from "@/components/terminal/restart-shell-bar";
import { useTerminalScreen } from "@/hooks/useTerminalScreen";

const TERMINAL_FONT_SIZE = 13;

/**
 * Terminal screen. Reuses a live PTY for the selected project (buffer replay
 * on return) or spawns one; leaving the screen keeps the PTY alive.
 * All state/effects live in `useTerminalScreen`; this file is composition.
 */
export function TerminalScreen() {
  const t = useTerminalScreen();

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title="Terminal"
        onBack={t.goBack}
        rightAction={
          t.term ? (
            <Pressable
              className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              onPress={t.handleKill}
            >
              <Trash2 size={18} color="#f87171" />
            </Pressable>
          ) : null
        }
      />

      {t.statusBanner ? (
        <View className="px-4 pb-2">
          <Text className="text-xs text-foreground-muted">{t.statusBanner}</Text>
        </View>
      ) : null}

      {/* Edge-to-edge Android never resizes the window for the IME. The whole
          column (surface + key bar) lives inside the avoiding view so the bar
          sits flush on the keyboard and the PTY reflows above it — otherwise
          the floating bar overlays the cursor row. */}
      <KeyboardAvoidingView behavior="padding" style={{ flex: 1 }}>
        <View className="flex-1 px-2 pb-1">
          {t.needsProjectPick ? (
            <ProjectPicker projects={t.projects} onSelect={t.selectProject} />
          ) : !t.project ? null : (
            // The surface mounts before the PTY exists: its first measured
            // size gates the spawn (true grid from the start). While spawning,
            // a spinner floats over the empty canvas.
            <View className="flex-1">
              <ConsoleTerminalSurface
                terminalKey={t.terminalId ?? ""}
                buffer={t.buffer}
                fontSize={TERMINAL_FONT_SIZE}
                isRunning={Boolean(t.isRunning)}
                autoFocus={Boolean(t.isRunning)}
                keyboardFocusRequest={t.focusRequest}
                keyboardDismissRequest={t.keyboardDismissRequest}
                onInput={t.handleInput}
                onResize={t.handleResize}
              />
              {!t.terminalId && !t.spawnError ? (
                <View
                  className="absolute inset-0 items-center justify-center gap-2"
                  pointerEvents="none"
                >
                  <ActivityIndicator />
                  <Text className="text-xs text-foreground-muted">Starting shell…</Text>
                </View>
              ) : null}
            </View>
          )}
        </View>

        {t.terminalId && t.isRunning ? (
          <ExtraKeysBar
            onShowKeyboard={t.requestFocus}
            onHideKeyboard={t.requestDismiss}
            onExtraKey={t.handleExtraKey}
          />
        ) : t.terminalId && !t.isRunning && t.term ? (
          <RestartShellBar onPress={t.handleKill} />
        ) : null}
      </KeyboardAvoidingView>
    </View>
  );
}
