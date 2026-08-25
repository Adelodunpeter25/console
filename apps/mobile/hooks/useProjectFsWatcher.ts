import { useEffect } from "react";
import { browseDirectory } from "@/stores/useFsStore";

/**
 * Real-time project filesystem watching.
 *
 * Polls the backend's FS browse endpoint for the project path and keeps the
 * fs store's `browse` state fresh. On mobile there's no OS-level watch relay
 * like the desktop's Rust `watch_directory`, so this uses a lightweight
 * interval poll while the hook is mounted. Stops on unmount.
 */
export function useProjectFsWatcher(projectPath?: string | null, pollMs = 5000) {

  useEffect(() => {
    if (!projectPath) return;

    let cancelled = false;
    const refresh = async () => {
      if (cancelled) return;
      try {
        await browseDirectory(projectPath);
      } catch {
        // Directory may not be browsable; ignore.
      }
    };

    void refresh();
    const interval = setInterval(() => {
      void refresh();
    }, pollMs);

    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [projectPath, pollMs]);
}
