import type { SqliteSessionStorage } from "@/agent/src/session/storage.js";

export function extractAndRecordFileChange(
  storage: SqliteSessionStorage,
  sessionId: string,
  toolName: string | undefined,
  args: any,
  isError?: boolean,
): void {
  if (isError || !toolName || !args) return;

  const now = Date.now();

  if ((toolName === "writeFile" || toolName === "write_file") && typeof args.path === "string") {
    const lineCount = typeof args.content === "string" ? args.content.split("\n").length : 0;
    storage.recordFileChange(sessionId, {
      path: args.path,
      status: "added",
      additions: lineCount,
      deletions: 0,
      turnIndex: 0,
      updatedAt: now,
    });
  } else if (
    (toolName === "editFile" || toolName === "edit_file" || toolName === "replace_file_content") &&
    (typeof args.path === "string" || typeof args.TargetFile === "string")
  ) {
    const targetPath = args.path || args.TargetFile;
    const adds =
      typeof args.replacement === "string"
        ? args.replacement.split("\n").length
        : typeof args.ReplacementContent === "string"
          ? args.ReplacementContent.split("\n").length
          : 1;
    const dels =
      typeof args.target === "string"
        ? args.target.split("\n").length
        : typeof args.TargetContent === "string"
          ? args.TargetContent.split("\n").length
          : 1;
    storage.recordFileChange(sessionId, {
      path: targetPath,
      status: "modified",
      additions: adds,
      deletions: dels,
      turnIndex: 0,
      updatedAt: now,
    });
  } else if ((toolName === "batchWrite" || toolName === "batch_write") && Array.isArray(args.files)) {
    for (const file of args.files) {
      if (typeof file.path === "string") {
        const lineCount = typeof file.content === "string" ? file.content.split("\n").length : 0;
        storage.recordFileChange(sessionId, {
          path: file.path,
          status: "added",
          additions: lineCount,
          deletions: 0,
          turnIndex: 0,
          updatedAt: now,
        });
      }
    }
  }
}
