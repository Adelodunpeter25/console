import type { AgentMessage } from "@/agent/src/types/index.js";
import { extractFileOps, formatFileTree } from "./file-tracker.js";

/** Max characters for user prompts in structural summary. */
const MAX_PROMPT_CHARS = 300;
/** Longer budget for the very first user prompt — it states the original goal. */
const MAX_FIRST_PROMPT_CHARS = 600;
/** Max characters for tool arg summaries in structural summary. */
const MAX_ARG_CHARS = 100;
/** Overall cap on the highlights section of the structural summary. */
const MAX_HIGHLIGHTS_CHARS = 3_000;
/** Max files with preserved content snippets in the file-facts section. */
const MAX_FILE_FACTS = 5;
/** Max characters kept per file fact snippet. */
const MAX_FILE_FACT_CHARS = 400;

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
 * Bounded per-file content facts for reads in the discarded turns: path +
 * opening snippet of the latest successful result. Lets a resumed session
 * continue from preserved facts instead of blindly rereading every file.
 */
function formatFileFacts(messages: AgentMessage[]): string {
  const pathByCallId = new Map<string, string>();
  for (const msg of messages) {
    if (msg.role !== "assistant") continue;
    for (const part of msg.content) {
      if (part.type !== "toolCall" || !part.call.name.toLowerCase().includes("read")) continue;
      const args = (part.call.arguments ?? {}) as Record<string, unknown>;
      const rawPath = args.path ?? args.filePath ?? args.targetFile;
      if (typeof rawPath === "string" && rawPath.trim()) {
        pathByCallId.set(part.call.id, rawPath.trim());
      }
    }
  }

  const facts: string[] = [];
  const seen = new Set<string>();
  for (const msg of messages) {
    if (msg.role !== "toolResult") continue;
    for (const res of msg.results) {
      if (facts.length >= MAX_FILE_FACTS) break;
      const path = pathByCallId.get(res.toolCallId);
      if (!path || seen.has(path) || res.isError) continue;
      seen.add(path);
      const text =
        typeof res.content === "string"
          ? res.content
          : (() => {
              try {
                return JSON.stringify(res.content) ?? "";
              } catch {
                return "";
              }
            })();
      if (!text.trim()) continue;
      const snippet =
        text.length > MAX_FILE_FACT_CHARS ? text.slice(0, MAX_FILE_FACT_CHARS) + "…" : text;
      facts.push(`## ${path}\n${snippet}`);
    }
  }

  if (facts.length === 0) return "";
  return `<file-facts>\n${facts.join("\n\n")}\n</file-facts>`;
}

/**
 * Build a deterministic, rich structural summary of discarded conversation turns.
 */
export function buildStructuralSummary(messages: AgentMessage[]): string {
  const highlights: string[] = [];
  let userTurnsCount = 0;
  let toolCallsCount = 0;

  // Pre-pass: frequency of each normalized user request, so retries of the
  // same prompt collapse to one line with a count instead of spamming the log.
  const requestFrequency = new Map<string, number>();
  for (const msg of messages) {
    if (msg.role !== "user") continue;
    const text = (msg.content?.trim() || "").replace(/\n+/g, " ");
    if (text) requestFrequency.set(text, (requestFrequency.get(text) ?? 0) + 1);
  }
  const reportedRequests = new Set<string>();
  let firstUserSeen = false;

  for (const msg of messages) {
    if (msg.role === "user") {
      userTurnsCount++;
      const text = msg.content?.trim() || "";
      if (text) {
        const flat = text.replace(/\n+/g, " ");
        if (reportedRequests.has(flat)) continue;
        reportedRequests.add(flat);
        const repeats = requestFrequency.get(flat) ?? 1;
        // The original goal gets a longer budget; repeats get a count suffix.
        const budget = !firstUserSeen ? MAX_FIRST_PROMPT_CHARS : MAX_PROMPT_CHARS;
        const truncated = flat.length > budget ? flat.slice(0, budget) + "…" : flat;
        highlights.push(
          `- User requested${repeats > 1 ? ` (${repeats}x)` : ""}: "${truncated}"`,
        );
      }
      firstUserSeen = true;
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
      const text = msg.content
        .filter((part): part is Extract<typeof part, { type: "text" | "thinking" }> => part.type === "text" || part.type === "thinking")
        .map((part) => part.text.trim())
        .filter(Boolean)
        // Provider failure boilerplate ("Error: ... overloaded ...") is not a
        // conclusion — drop it so retries don't read as decisions.
        .filter((t) => !/^\s*error:/i.test(t))
        .join(" ");
      if (text) {
        const truncated = text.length > MAX_PROMPT_CHARS ? text.slice(0, MAX_PROMPT_CHARS) + "…" : text;
        highlights.push(`- Assistant concluded: "${truncated.replace(/\n+/g, " ")}"`);
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

  const fileFacts = formatFileFacts(messages);
  if (fileFacts) {
    parts.push(``, fileFacts);
  }

  return parts.join("\n");
}
