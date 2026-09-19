import type { SqliteSessionStorage } from "@/agent/src/session/storage.js";
import { createPatch } from "diff";

const MAX_DIFF_SIZE = 1_000_000; // 1MB limit for diff storage

export function extractAndRecordFileChange(
  storage: SqliteSessionStorage,
  sessionId: string,
  toolName: string | undefined,
  args: any,
  isError?: boolean,
  turnIndex?: number,
): void {
  if (isError || !toolName || !args) return;

  const now = Date.now();
  const currentTurnIndex = turnIndex ?? 0;

  if ((toolName === "writeFile" || toolName === "write_file") && typeof args.path === "string") {
    const content = typeof args.content === "string" ? args.content : "";
    const lineCount = content.split("\n").length;

    // Generate diff for new file (against empty file)
    let diffText: string | undefined;
    try {
      const patch = createPatch(args.path, "", content);
      if (patch.length <= MAX_DIFF_SIZE) {
        diffText = patch;
      }
    } catch (error) {
      console.error("Failed to generate diff for writeFile:", error);
    }

    storage.recordFileChange(sessionId, {
      path: args.path,
      status: "added",
      additions: lineCount,
      deletions: 0,
      turnIndex: currentTurnIndex,
      diffText,
      updatedAt: now,
    });
  } else if (
    (toolName === "editFile" || toolName === "edit_file" || toolName === "replace_file_content") &&
    (typeof args.path === "string" || typeof args.TargetFile === "string")
  ) {
    const targetPath = args.path || args.TargetFile;
    const targetContent =
      typeof args.target === "string"
        ? args.target
        : typeof args.TargetContent === "string"
          ? args.TargetContent
          : "";
    const replacementContent =
      typeof args.replacement === "string"
        ? args.replacement
        : typeof args.ReplacementContent === "string"
          ? args.ReplacementContent
          : "";

    const adds = replacementContent.split("\n").length;
    const dels = targetContent.split("\n").length;

    // Generate unified diff
    let diffText: string | undefined;
    try {
      const patch = createPatch(targetPath, targetContent, replacementContent);
      if (patch.length <= MAX_DIFF_SIZE) {
        diffText = patch;
      }
    } catch (error) {
      console.error("Failed to generate diff for editFile:", error);
    }

    storage.recordFileChange(sessionId, {
      path: targetPath,
      status: "modified",
      additions: adds,
      deletions: dels,
      turnIndex: currentTurnIndex,
      diffText,
      updatedAt: now,
    });
  } else if ((toolName === "batchWrite" || toolName === "batch_write") && Array.isArray(args.files)) {
    for (const file of args.files) {
      if (typeof file.path === "string") {
        const content = typeof file.content === "string" ? file.content : "";
        const lineCount = content.split("\n").length;

        // Generate diff for new file
        let diffText: string | undefined;
        try {
          const patch = createPatch(file.path, "", content);
          if (patch.length <= MAX_DIFF_SIZE) {
            diffText = patch;
          }
        } catch (error) {
          console.error("Failed to generate diff for batchWrite file:", error);
        }

        storage.recordFileChange(sessionId, {
          path: file.path,
          status: "added",
          additions: lineCount,
          deletions: 0,
          turnIndex: currentTurnIndex,
          diffText,
          updatedAt: now,
        });
      }
    }
  }
}
