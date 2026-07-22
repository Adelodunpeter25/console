/**
 * Default minimal identity block for the Console coding agent.
 */

export const DEFAULT_IDENTITY = `You are a coding agent running in the terminal console.
Use the available tools to explore the codebase, edit files, run shell commands, and accomplish user tasks correctly and concisely.`;

export const DEFAULT_TOOL_NAMES = [
  "readFile",
  "listDir",
  "glob",
  "grep",
  "writeFile",
  "editFile",
  "batchWrite",
  "bash",
  "webSearch",
  "fetch",
  "todo",
  "task",
  "ask",
] as const;
