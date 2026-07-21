export * from "./bash.js";
export * from "./batch-write.js";
export * from "./edit-file.js";
export * from "./fetch.js";
export * from "./glob.js";
export * from "./grep.js";
export * from "./list-dir.js";
export * from "./read-file.js";
export * from "./web-search.js";
export * from "./write-file.js";

import { bashTool } from "./bash.js";
import { batchWriteTool } from "./batch-write.js";
import { editFileTool } from "./edit-file.js";
import { fetchTool } from "./fetch.js";
import { globTool } from "./glob.js";
import { grepTool } from "./grep.js";
import { listDirTool } from "./list-dir.js";
import { readFileTool } from "./read-file.js";
import { webSearchTool } from "./web-search.js";
import { writeFileTool } from "./write-file.js";

/**
 * The full set of built-in tools.
 * Pass this (or a subset) into AgentLoopConfig.tools or Agent constructor.
 */
export const allTools = [
  // File system — read
  readFileTool,
  listDirTool,
  globTool,    // fff-powered (Rust native, warm-indexed)
  grepTool,    // fff-powered (Rust native, warm-indexed)
  // File system — write
  writeFileTool,
  editFileTool,
  batchWriteTool,
  // Execution
  bashTool,
  // Network
  webSearchTool,
  fetchTool,
] as const;
