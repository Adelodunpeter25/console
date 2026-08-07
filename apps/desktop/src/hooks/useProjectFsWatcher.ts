import React from "react";
import { useFsStore } from "../store/useFsStore";
import { tauriApi } from "../lib/tauri-api";

/**
 * Custom React hook encapsulating real-time project filesystem watching & auto-sync.
 * Decouples IPC listener setup, teardown, and directory browsing from UI components.
 */
export function useProjectFsWatcher(projectPath?: string | null) {
  const browseDirectory = useFsStore((state) => state.browseDirectory);

  React.useEffect(() => {
    if (!projectPath) return;

    // Initial browse fetch for the selected project path
    browseDirectory(projectPath).catch(() => {});

    let unlisten: (() => void) | undefined;

    // Trigger Rust command to watch project path via Tauri IPC
    tauriApi.watchDirectory(projectPath).catch(() => {});

    // Listen to fs-change Tauri event via Rust IPC relay
    tauriApi
      .listenFsChanges(() => {
        browseDirectory(projectPath).catch(() => {});
      })
      .then((unlistenFn) => {
        unlisten = unlistenFn;
      })
      .catch(() => {});

    return () => {
      if (unlisten) unlisten();
    };
  }, [projectPath, browseDirectory]);
}
