/**
 * Default minimal identity block for the Console coding agent.
 */

export const DEFAULT_IDENTITY = `You are a coding agent.
Use the available tools to explore the codebase, edit files, run shell commands, and accomplish user tasks correctly and concisely.`;

export const DEFAULT_TOOL_NAMES = [
  "readFile",
  "readSkill",
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
  "subagent",
  "ask",
] as const;
