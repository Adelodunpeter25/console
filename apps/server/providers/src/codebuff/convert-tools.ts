/**
 * Codebuff tool converter (OpenAI chat-completions tool format).
 *
 * The freebuff server rejects free-mode requests whose offered tools contain
 * none of its signature tool names (`foreign_toolset` downgrade in
 * foreign-client-signals.ts). We therefore send our tools under freebuff's
 * signature names (via CODEBUFF_TOOL_ALIASES) and reverse-map the names back to
 * the console's own tool ids in the stream-fn so calls still dispatch to the
 * right implementation.
 */
import type { AgentTool } from "@console/types";
import type { ToolSet } from "ai";
import { zodToJsonSchema } from "zod-to-json-schema";

/** Console tool id → freebuff signature tool name (injective). */
export const CODEBUFF_TOOL_ALIASES: Record<string, string> = {
  bash: "run_terminal_command",
  readFile: "read_files",
  writeFile: "propose_write_file",
  editFile: "str_replace",
  glob: "find_files",
  grep: "code_search",
  listDir: "list_directory",
  fetch: "read_url",
  ask: "ask_user",
  subagent: "spawn_agents",
  todo: "write_todos",
};

/** Freebuff signature name → console tool id (inverse of CODEBUFF_TOOL_ALIASES). */
export const CODEBUFF_TOOL_ALIAS_REVERSE: Record<string, string> =
  Object.fromEntries(
    Object.entries(CODEBUFF_TOOL_ALIASES).map(([consoleName, freebuffName]) => [
      freebuffName,
      consoleName,
    ]),
  );

/** Freebuff signature tool name for a console tool, or the console name. */
export function freebuffToolNameFor(consoleName: string): string {
  return CODEBUFF_TOOL_ALIASES[consoleName] ?? consoleName;
}

/** Console tool id for a freebuff signature tool name, or the freebuff name. */
export function consoleToolNameFor(freebuffName: string): string {
  return CODEBUFF_TOOL_ALIAS_REVERSE[freebuffName] ?? freebuffName;
}

export function convertCodebuffTools(tools: AgentTool[]): ToolSet {
  const result: ToolSet = {};

  for (const tool of tools) {
    result[freebuffToolNameFor(tool.name)] = {
      description: tool.description,
      parameters: zodToJsonSchema(tool.inputSchema, {
        target: "openApi3",
        $refStrategy: "none",
      }) as any,
    };
  }

  return result;
}