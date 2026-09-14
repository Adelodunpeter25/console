/**
 * LLM compaction summaries via the configured smol model.
 *
 * This is the default summary path (`summaryStrategy: "llm"`). The hook
 * resolves the smol role, feeds it a bounded plain-text transcript of the
 * discarded turns, and returns the narrative summary. Single attempt, no
 * retries, no nested compaction. When no smol model is configured (or the
 * call fails), it throws so the agent loop falls back to structural.
 */
import type { AgentMessage, Model } from "@/agent/src/types/index.js";
import type { StreamFn } from "@/agent/src/service/types.js";
import { hasConfiguredRole, resolveModelRole } from "@/agent/src/service/role-resolver.js";

/** Hard cap on transcript characters sent to the summarizer. */
export const SUMMARY_INPUT_MAX_CHARS = 60_000;
/** Per-line cap so one huge tool dump can't eat the whole budget. */
const SUMMARY_LINE_MAX_CHARS = 2_000;

const DEFAULT_SUMMARY_SYSTEM_PROMPT =
  "Summarize the prior coding conversation for future continuation. Preserve decisions, files, changes, unresolved work, and important constraints. Return only the summary.";

function argSnippet(args: unknown): string {
  if (args == null) return "";
  let text: string;
  try {
    text = typeof args === "string" ? args : (JSON.stringify(args) ?? "");
  } catch {
    return "";
  }
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > 200 ? `${flat.slice(0, 200)}…` : flat;
}

function sliceLine(text: string): string {
  const flat = text.replace(/\s+/g, " ").trim();
  return flat.length > SUMMARY_LINE_MAX_CHARS ? `${flat.slice(0, SUMMARY_LINE_MAX_CHARS)}…` : flat;
}

/**
 * Flatten discarded turns into transcript lines, oldest first. The retained
 * tail already carries recent context, so when the budget runs out the newer
 * end of the discarded prefix is what gets omitted.
 */
export function buildSummaryTranscript(messages: AgentMessage[]): string[] {
  const lines: string[] = [];
  for (const msg of messages) {
    if (msg.role === "user") {
      if (msg.content?.trim()) lines.push(`[user] ${sliceLine(msg.content)}`);
      for (const att of msg.attachments ?? []) {
        lines.push(`[user] (image attached: ${att.mimeType})`);
      }
    } else if (msg.role === "assistant") {
      for (const part of msg.content) {
        if (part.type === "text" && part.text.trim()) {
          lines.push(`[assistant] ${sliceLine(part.text)}`);
        } else if (part.type === "thinking" && part.text.trim()) {
          lines.push(`[assistant thinking] ${sliceLine(part.text)}`);
        } else if (part.type === "toolCall") {
          const args = argSnippet(part.call.arguments);
          lines.push(`[tool:${part.call.name}]${args ? ` ${args}` : ""}`);
        }
      }
    } else {
      for (const res of msg.results) {
        const raw =
          typeof res.content === "string"
            ? res.content
            : (() => {
                try {
                  return JSON.stringify(res.content) ?? "";
                } catch {
                  return "";
                }
              })();
        if (raw.trim()) lines.push(`[result${res.isError ? " (error)" : ""}] ${sliceLine(raw)}`);
      }
    }
  }
  return lines;
}

/**
 * Pack transcript lines into a single user message within the char budget,
 * keeping the oldest lines. A flat single-message input keeps every provider
 * wire format happy (no tool-pair or role-alternation constraints).
 */
export function packSummaryInput(
  messages: AgentMessage[],
  maxChars: number = SUMMARY_INPUT_MAX_CHARS,
): AgentMessage[] {
  const kept: string[] = [];
  let total = 0;
  for (const line of buildSummaryTranscript(messages)) {
    if (total + line.length + 1 > maxChars) {
      kept.push("[... newer content omitted for length ...]");
      break;
    }
    kept.push(line);
    total += line.length + 1;
  }
  if (kept.length === 0) {
    kept.push("(no prior content)");
  }
  return [{ role: "user", content: kept.join("\n") }];
}

export interface SmolSummarizerOptions {
  /** Session model used when no smol role resolves (never happens — missing role throws first). */
  getFallbackModel: () => Model;
  /** Transport for the resolved smol model. */
  getStreamFn: (model: Model) => StreamFn;
  hasSmolRole?: () => Promise<boolean>;
  resolveSmol?: (fallback: Model) => Promise<Model>;
  maxInputChars?: number;
  systemPrompt?: string;
}

/**
 * Build the `summarizeCompaction` hook used with `summaryStrategy: "llm"`.
 * Throws when no smol model is configured or the summary comes back empty —
 * the agent loop treats either as "use the structural summary".
 */
export function createSmolSummarizer(options: SmolSummarizerOptions) {
  const hasSmolRole = options.hasSmolRole ?? (() => hasConfiguredRole("smol"));
  const resolveSmol = options.resolveSmol ?? ((fallback: Model) => resolveModelRole("smol", fallback));
  const maxInputChars = options.maxInputChars ?? SUMMARY_INPUT_MAX_CHARS;
  const systemPrompt = options.systemPrompt ?? DEFAULT_SUMMARY_SYSTEM_PROMPT;

  return async (messages: AgentMessage[], signal?: AbortSignal): Promise<string> => {
    if (!(await hasSmolRole())) throw new Error("No smol model configured");
    const smolModel = await resolveSmol(options.getFallbackModel());
    const streamFn = options.getStreamFn(smolModel);
    const input = packSummaryInput(messages, maxInputChars);

    let summary = "";
    for await (const delta of streamFn({
      model: smolModel,
      systemPrompt,
      messages: input,
      tools: [],
      signal,
    })) {
      if (delta.type === "text") summary += delta.text;
    }
    if (!summary.trim()) throw new Error("Empty compaction summary");
    return summary.trim();
  };
}
