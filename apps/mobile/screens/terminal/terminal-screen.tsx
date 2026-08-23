import React, { useCallback, useEffect, useRef, useState } from "react";
import { Text, View, Pressable, BackHandler, ActivityIndicator } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { ChevronLeft, Keyboard, KeyboardOff, Trash2 } from "lucide-react-native";
import { KeyboardAvoidingView, useKeyboardState } from "react-native-keyboard-controller";
import { ScreenHeader } from "../../components/layout/screen-header";
import { useAppStore, useProjectStore, useTerminalStore } from "../../stores";
import { ConsoleTerminalSurface } from "../../modules/console-terminal/src/terminal-surface";

const TERMINAL_FONT_SIZE = 13;

/** Extra keys sent verbatim to the PTY. */
const EXTRA_KEYS: { label: string; bytes: string }[] = [
  { label: "Esc", bytes: "\u001B" },
  { label: "Tab", bytes: "\t" },
  { label: "↑", bytes: "\u001B[A" },
  { label: "↓", bytes: "\u001B[B" },
  { label: "←", bytes: "\u001B[D" },
  { label: "→", bytes: "\u001B[C" },
  { label: "Ctrl-C", bytes: "\u0003" },
];

/**
 * Terminal screen. Reuses a live PTY for the selected project (buffer replay
 * on return) or spawns one; leaving the screen keeps the PTY alive.
 */
export function TerminalScreen() {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const previousTab = useAppStore((state) => state.previousTab);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const selectedProjectId = useAppStore((state) => state.selectedProjectId);
  const setSelectedProjectId = useAppStore((state) => state.setSelectedProjectId);
  const projects = useProjectStore((state) => state.projects);

  const [terminalId, setTerminalId] = useState<string | null>(null);
  const [spawnError, setSpawnError] = useState<string | null>(null);
  const [focusRequest, setFocusRequest] = useState(0);
  const [keyboardDismissRequest, setKeyboardDismissRequest] = useState(0);
  const resizeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  // Drop a pending debounced PTY resize when the screen unmounts.
  useEffect(() => () => clearTimeout(resizeTimer.current), []);

  // Strict lookup only: a dangling/deleted selection must never silently open
  // another project's working directory.
  const project = selectedProjectId
    ? projects.find((p) => p.id === selectedProjectId) ?? null
    : null;

  const needsProjectPick = !project && projects.length > 0;

  // Back returns to wherever the user came from (never terminal itself).
  const goBack = useCallback(() => {
    setActiveTab(previousTab && previousTab !== "terminal" ? previousTab : "home");
  }, [previousTab, setActiveTab]);

  // The keyboard button toggles: visible → dismiss+blur, hidden → focus+show.
  const toggleKeyboard = useCallback(() => {
    if (keyboardVisible) {
      setKeyboardDismissRequest((n) => n + 1);
    } else {
      setFocusRequest((n) => n + 1);
    }
  }, [keyboardVisible]);

  // Spawn or reuse on mount (and if the selected project changes while mounted).
  useEffect(() => {
    if (!project) return;
    let cancelled = false;

    (async () => {
      const store = useTerminalStore.getState();
      const existing = store.findLiveTerminal(project.id, project.path);
      if (existing) {
        setTerminalId(existing);
        return;
      }
      try {
        const spawned = await store.openTerminal({
          projectId: project.id,
          cwd: project.path,
        });
        if (!cancelled) setTerminalId(spawned.id);
      } catch (cause) {
        if (!cancelled) {
          setSpawnError(cause instanceof Error ? cause.message : String(cause));
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [project?.id, project?.path]);

  useEffect(() => {
    const onBackPress = () => {
      goBack();
      return true;
    };
    const sub = BackHandler.addEventListener("hardwareBackPress", onBackPress);
    return () => sub.remove();
  }, [goBack]);

  const term = useTerminalStore((state) => (terminalId ? state.terminals[terminalId] : undefined));
  const buffer = useTerminalStore((state) => (terminalId ? (state.buffers[terminalId] ?? "") : ""));

  const handleInput = useCallback(
    (data: string) => {
      if (terminalId) useTerminalStore.getState().write(terminalId, data);
      // Keep the soft keyboard up after sending a command; otherwise the IME
      // hides on IME_ACTION_SEND and the user must tap the terminal again.
      if (data.includes("\r") || data.includes("\n")) {
        // Delay a tick so the native side's onEditorAction can finish its hide
        // animation before we re-assert focus.
        setTimeout(() => setFocusRequest((n) => n + 1), 40);
      }
    },
    [terminalId],
  );

  const handleExtraKey = useCallback(
    (bytes: string) => {
      handleInput(bytes);
      // Extra keys live outside the native EditText; without an explicit focus
      // request the inputView loses focus and the terminal flicks to its
      // unfocused background until the user taps again.
      setFocusRequest((n) => n + 1);
    },
    [handleInput],
  );

  // Coalesce resize storms (rotation, keyboard) into one PTY resize.
  const handleResize = useCallback(
    (size: { cols: number; rows: number }) => {
      if (!terminalId) return;
      if (resizeTimer.current) clearTimeout(resizeTimer.current);
      resizeTimer.current = setTimeout(() => {
        useTerminalStore.getState().resize(terminalId, size.cols, size.rows);
      }, 100);
    },
    [terminalId],
  );

  const handleKill = useCallback(() => {
    if (!terminalId || !project) return;
    const idToKill = terminalId;
    // Clear UI first so the spawn spinner shows while the old PTY tears down
    setTerminalId(null);
    setSpawnError(null);
    useTerminalStore
      .getState()
      .kill(idToKill)
      .catch(() => {})
      .finally(() => {
        // Respawn a fresh PTY for the same project so the user lands back in a shell
        useTerminalStore
          .getState()
          .openTerminal({ projectId: project.id, cwd: project.path })
          .then((spawned) => setTerminalId(spawned.id))
          .catch((cause) => setSpawnError(cause instanceof Error ? cause.message : String(cause)));
      });
  }, [terminalId, project]);

  const isRunning = term?.status === "running" || term?.status === "spawning";

  // Auto-focus after spawn so the prompt is immediately typable without a tap.
  useEffect(() => {
    if (!terminalId || !isRunning) return;
    const timer = setTimeout(() => setFocusRequest((n) => n + 1), 350);
    return () => clearTimeout(timer);
  }, [terminalId, isRunning]);

  const statusBanner = (() => {
    if (spawnError) return `Failed to start terminal: ${spawnError}`;
    if (!project) return projects.length === 0 ? "No projects yet — add a project first." : null;
    if (term?.status === "exited") return "Session ended.";
    if (term?.status === "error") return `Session error: ${term.error ?? "unknown"}`;
    return null;
  })();

  return (
    <View className="flex-1 bg-screen">
      <ScreenHeader
        title="Terminal"
        onBack={goBack}
        rightAction={
          term ? (
            <Pressable
              className="w-10 h-10 rounded-full bg-card border border-border items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              onPress={handleKill}
            >
              <Trash2 size={18} color="#f87171" />
            </Pressable>
          ) : null
        }
      />

      {statusBanner ? (
        <View className="px-4 pb-2">
          <Text className="text-xs text-foreground-muted">{statusBanner}</Text>
        </View>
      ) : null}

      {/* Edge-to-edge Android never resizes the window for the IME. The whole
          column (surface + key bar) lives inside the avoiding view so the bar
          sits flush on the keyboard and the PTY reflows above it — otherwise
          the floating bar overlays the cursor row. */}
      <KeyboardAvoidingView behavior="padding" style={{ flex: 1 }}>
        <View className="flex-1 px-2 pb-1">
        {needsProjectPick ? (
          <View className="flex-1 items-center justify-center gap-2 px-6">
            <Text className="text-xs text-foreground-muted">
              Select a project for this terminal
            </Text>
            {projects.map((p) => (
              <Pressable
                key={p.id}
                className="w-full max-w-sm rounded-lg bg-card border border-border px-4 py-2.5 gap-0.5"
                style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                onPress={() => setSelectedProjectId(p.id)}
              >
                <Text className="text-sm font-semibold text-foreground">{p.name}</Text>
                <Text className="text-xs font-mono text-foreground-muted" numberOfLines={1}>
                  {p.path}
                </Text>
              </Pressable>
            ))}
          </View>
        ) : !project ? null : !terminalId && !spawnError ? (
          <View className="flex-1 items-center justify-center gap-2">
            <ActivityIndicator />
            <Text className="text-xs text-foreground-muted">Starting shell…</Text>
          </View>
        ) : terminalId ? (
          <ConsoleTerminalSurface
            terminalKey={terminalId}
            buffer={buffer}
            fontSize={TERMINAL_FONT_SIZE}
            isRunning={Boolean(isRunning)}
            keyboardFocusRequest={focusRequest}
            keyboardDismissRequest={keyboardDismissRequest}
            onInput={handleInput}
            onResize={handleResize}
          />
        ) : null}
      </View>

      {terminalId && isRunning ? (
          <View
            className="flex-row items-center gap-1.5 px-2 pt-1.5 border-t border-border-subtle bg-screen"
            style={{ paddingBottom: keyboardVisible ? 6 : insets.bottom + 6 }}
          >
            <Pressable
              className="h-8 px-2.5 rounded-md bg-card border border-border items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              onPress={toggleKeyboard}
            >
              {keyboardVisible ? (
                <KeyboardOff size={15} color="#a1a1aa" />
              ) : (
                <Keyboard size={15} color="#a1a1aa" />
              )}
            </Pressable>
            {EXTRA_KEYS.map((key) => (
              <Pressable
                key={key.label}
                className="h-8 min-w-9 px-2 rounded-md bg-card border border-border items-center justify-center"
                style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                onPress={() => handleExtraKey(key.bytes)}
              >
                <Text className="text-xs font-mono text-foreground-secondary">{key.label}</Text>
              </Pressable>
            ))}
          </View>
      ) : terminalId && !isRunning && term ? (
          <View
            className="flex-row items-center justify-center px-4 pt-3 border-t border-border-subtle bg-screen"
            style={{ paddingBottom: keyboardVisible ? 10 : insets.bottom + 10 }}
          >
            <Pressable
              className="px-4 py-2.5 rounded-full bg-foreground items-center justify-center"
              style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
              onPress={handleKill}
            >
              <Text className="text-xs font-semibold text-background">Restart shell</Text>
            </Pressable>
          </View>
      ) : null}
      </KeyboardAvoidingView>
    </View>
  );
}
