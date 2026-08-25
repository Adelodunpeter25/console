import fs from "node:fs";
import path from "node:path";
import { EventEmitter } from "node:events";
import { isPathIgnored } from "@/api/src/utils/ignored.js";

export interface FsWatcherEvent {
  type: "fsChange";
  projectPath: string;
  eventPath?: string;
}

export class FsWatchService extends EventEmitter {
  private watchers = new Map<string, fs.FSWatcher>();
  private debounceTimers = new Map<string, ReturnType<typeof setTimeout>>();

  /**
   * Start watching a project directory for real-time filesystem events.
   */
  watch(projectPath: string): void {
    const normPath = path.resolve(projectPath);
    if (this.watchers.has(normPath)) return;

    try {
      const watcher = fs.watch(normPath, { recursive: true }, (eventType, filename) => {
        const relativeFilename = filename ? filename.toString() : "";
        if (relativeFilename && isPathIgnored(relativeFilename)) {
          return;
        }

        // Debounce notifications per project path (300ms) to avoid event spamming
        if (this.debounceTimers.has(normPath)) {
          clearTimeout(this.debounceTimers.get(normPath)!);
        }

        const timer = setTimeout(() => {
          this.debounceTimers.delete(normPath);
          const fullEventPath = filename ? path.join(normPath, relativeFilename) : undefined;
          this.emit("change", {
            type: "fsChange",
            projectPath: normPath,
            eventPath: fullEventPath,
          } as FsWatcherEvent);
        }, 300);

        this.debounceTimers.set(normPath, timer);
      });

      this.watchers.set(normPath, watcher);
    } catch (err) {
      console.warn(`[FsWatchService] Failed to watch ${normPath}:`, err);
    }
  }

  /**
   * Stop watching a project directory.
   */
  unwatch(projectPath: string): void {
    const normPath = path.resolve(projectPath);
    const watcher = this.watchers.get(normPath);
    if (watcher) {
      watcher.close();
      this.watchers.delete(normPath);
    }
    if (this.debounceTimers.has(normPath)) {
      clearTimeout(this.debounceTimers.get(normPath)!);
      this.debounceTimers.delete(normPath);
    }
  }
}

export const fsWatchService = new FsWatchService();
