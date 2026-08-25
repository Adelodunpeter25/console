import { useCallback, useEffect, useRef, useState } from "react";
import { BackHandler } from "react-native";
import { useKeyboardState } from "react-native-keyboard-controller";
import { useAppStore, useProjectStore, useTerminalStore } from "@/stores";

/**
 * All terminal-screen state and side effects: project scoping, PTY spawn/reuse/
 * kill lifecycle, input plumbing, resize debouncing, and IME focus control.
 * The screen file stays pure composition.
 */
export function useTerminalScreen() {
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
  /** Latest grid measured by the terminal surface. The first measurement gates
   * the PTY spawn so the shell starts at the true size, never the 80×24 default. */
  const surfaceSizeRef = useRef<{ cols: number; rows: number } | null>(null);
  const [hasSurfaceSize, setHasSurfaceSize] = useState(false);

  const keyboardVisible = useKeyboardState((s) => s.isVisible);

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

  // Spawn or reuse on mount — but only once the surface reports its first
  // size. The measured cols/rows flow into openTerminal (and the WS URL) so
  // the PTY is created at the true grid and the shell's first prompt draws at
  // the correct width instead of the 80×24 default.
  useEffect(() => {
    if (!project || !hasSurfaceSize) return;
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
          cols: surfaceSizeRef.current?.cols,
          rows: surfaceSizeRef.current?.rows,
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
  }, [project?.id, project?.path, hasSurfaceSize]);

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

  // Pre-spawn: track the measured grid; the first report unblocks the spawn
  // effect. Post-spawn: coalesce resize storms (rotation, keyboard) into one
  // debounced PTY resize.
  const handleResize = useCallback(
    (size: { cols: number; rows: number }) => {
      if (!terminalId) {
        const first = surfaceSizeRef.current === null;
        surfaceSizeRef.current = size;
        if (first) setHasSurfaceSize(true);
        return;
      }
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
        // Respawn a fresh PTY for the same project so the user lands back in a
        // shell, reusing the last measured grid for the spawn params.
        const size = surfaceSizeRef.current ?? undefined;
        useTerminalStore
          .getState()
          .openTerminal({
            projectId: project.id,
            cwd: project.path,
            cols: size?.cols,
            rows: size?.rows,
          })
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

  const requestFocus = useCallback(() => setFocusRequest((n) => n + 1), []);
  const requestDismiss = useCallback(() => setKeyboardDismissRequest((n) => n + 1), []);

  const selectProject = useCallback(
    (projectId: string) => setSelectedProjectId(projectId),
    [setSelectedProjectId],
  );

  const statusBanner = (() => {
    if (spawnError) return `Failed to start terminal: ${spawnError}`;
    if (!project) return projects.length === 0 ? "No projects yet — add a project first." : null;
    if (term?.status === "exited") return "Session ended.";
    if (term?.status === "error") return `Session error: ${term.error ?? "unknown"}`;
    return null;
  })();

  return {
    projects,
    project,
    needsProjectPick,
    selectProject,
    statusBanner,
    terminalId,
    term,
    buffer,
    isRunning,
    spawnError,
    focusRequest,
    keyboardDismissRequest,
    keyboardVisible,
    goBack,
    handleKill,
    handleInput,
    handleExtraKey,
    handleResize,
    requestFocus,
    requestDismiss,
  };
}
