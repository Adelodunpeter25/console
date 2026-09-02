import type { AgentMessage } from "@/agent/src/types/index.js";
import { extractFileOps, formatFileTree } from "./file-tracker.js";

/** Max characters for user prompts in structural summary. */
const MAX_PROMPT_CHARS = 300;
/** Max characters for tool arg summaries in structural summary. */
const MAX_ARG_CHARS = 100;
/** Overall cap on the highlights section of the structural summary. */
const MAX_HIGHLIGHTS_CHARS = 3_000;

function formatToolArgs(args: unknown): string {
  if (!args || typeof args !== "object") return "";
  const obj = args as Record<string, unknown>;
  const keyVal =
    obj.path ||
    obj.filePath ||
    obj.targetFile ||
    obj.TargetFile ||
    obj.absolutePath ||
    obj.AbsolutePath ||
    obj.command ||
    obj.CommandLine ||
    obj.query ||
    obj.Query ||
    obj.pattern ||
    obj.Pattern;

  if (keyVal != null) {
    const s = String(keyVal);
    return s.length > MAX_ARG_CHARS ? s.slice(0, MAX_ARG_CHARS) + "…" : s;
  }
  return "";
}

/**
 * Build a deterministic, rich structural summary of discarded conversation turns.
 */
export function buildStructuralSummary(messages: AgentMessage[]): string {
  const highlights: string[] = [];
  let userTurnsCount = 0;
  let toolCallsCount = 0;

  for (const msg of messages) {
    if (msg.role === "user") {
      userTurnsCount++;
      const text = msg.content?.trim() || "";
      if (text) {
        const truncated = text.length > MAX_PROMPT_CHARS ? text.slice(0, MAX_PROMPT_CHARS) + "…" : text;
        highlights.push(`- User requested: "${truncated.replace(/\n+/g, " ")}"`);
      }
    } else if (msg.role === "assistant") {
      const calls: string[] = [];
      for (const part of msg.content) {
        if (part.type === "toolCall") {
          toolCallsCount++;
          const summary = formatToolArgs(part.call.arguments);
          calls.push(summary ? `${part.call.name}(${summary})` : part.call.name);
        }
      }
      if (calls.length > 0) {
        highlights.push(`- Executed: ${calls.slice(0, 4).join(", ")}${calls.length > 4 ? ` (+${calls.length - 4} more)` : ""}`);
      }
    } else if (msg.role === "toolResult") {
      for (const res of msg.results) {
        if (res.isError) {
          const errText = typeof res.content === "string" ? res.content.slice(0, 80) : "Failed";
          highlights.push(`  ↳ [error]: ${errText.replace(/\n+/g, " ")}`);
        }
      }
    }
  }

  // Deduplicate consecutive identical lines
  const deduped: string[] = [];
  for (const line of highlights) {
    if (deduped.length === 0 || deduped[deduped.length - 1] !== line) {
      deduped.push(line);
    }
  }

  let highlightsText = deduped.join("\n");
  if (highlightsText.length > MAX_HIGHLIGHTS_CHARS) {
    highlightsText = highlightsText.slice(0, MAX_HIGHLIGHTS_CHARS) + "\n[…prior highlights truncated…]";
  }

  const fileOps = extractFileOps(messages);
  const fileTree = formatFileTree(fileOps);

  const parts = [
    `Prior model work/tool state available.`,
    `MUST build on prior work; NEVER duplicate prior work.`,
    ``,
    `<summary>`,
    `[Conversation Checkpoint: Compacted ${messages.length} messages (${userTurnsCount} user requests, ${toolCallsCount} tool operations)]`,
    ``,
    `Key Actions & Context:`,
    highlightsText || `- Completed preliminary exploratory operations.`,
    `</summary>`,
  ];

  if (fileTree) {
    parts.push(``, fileTree);
  }

  return parts.join("\n");
}
