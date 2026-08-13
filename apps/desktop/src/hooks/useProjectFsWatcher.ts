import React from "react";
import { useFsStore } from "../store/useFsStore";
import { api } from "../lib/api";

/**
 * Custom React hook encapsulating real-time project filesystem watching & auto-sync.
 * Decouples IPC listener setup, teardown, and directory browsing from UI components.
 */
export function useProjectFsWatcher(projectPath?: string | null) {
  const activePathRef = React.useRef(projectPath);
  activePathRef.current = projectPath;

  React.useEffect(() => {
    if (!projectPath) return;

    // Initial browse fetch for the selected project path
    useFsStore.getState().browseDirectory(projectPath).catch(() => {});

    let unlisten: (() => void) | undefined;
    let debounceTimer: ReturnType<typeof setTimeout> | undefined;

    // Trigger Rust command to watch project path via Tauri IPC
    api.watchDirectory(projectPath).catch(() => {});

    // Listen to fs-change Tauri event via Rust IPC relay with 300ms debouncing
    api
      .listenFsChanges(() => {
        if (debounceTimer) clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => {
          const currentPath = activePathRef.current;
          if (currentPath) {
            useFsStore.getState().browseDirectory(currentPath).catch(() => {});
          }
        }, 300);
      })
      .then((unlistenFn) => {
        unlisten = unlistenFn;
      })
      .catch(() => {});

    return () => {
      if (debounceTimer) clearTimeout(debounceTimer);
      if (unlisten) unlisten();
    };
  }, [projectPath]);
}
