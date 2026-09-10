import type { Model } from "@console/types";
import { resolveModelRole } from "./role-resolver.js";
import type { StreamFn } from "./agent-loop.js";

export function isGenericSessionTitle(title: string | undefined): boolean {
  const value = title?.trim();
  return !value || ["New Session", "New mobile session", "New Chat", "New chat", "Untitled"].includes(value);
}

export function fallbackSessionTitle(prompt: string): string {
  const compact = prompt.replace(/\s+/g, " ").trim();
  return compact.length > 35 ? `${compact.slice(0, 35)}...` : compact;
}

export function sanitizeSessionTitle(value: string): string {
  const singleLine = value.replace(/[\r\n]+/g, " ").replace(/^\s*[-*#]+\s*/, "").replace(/^["'`]+|["'`.]+$/g, "");
  const words = singleLine.trim().split(/\s+/).filter(Boolean).slice(0, 8);
  return words.join(" ").slice(0, 80).trim();
}

export async function generateSessionTitle(prompt: string, model: Model, streamFn: StreamFn): Promise<string | null> {
  const titleModel = await resolveModelRole("smol", model);
  let output = "";
  for await (const delta of streamFn({
    model: titleModel,
    systemPrompt: "Create a concise session title from the user's request. Output one single-line sentence of about six words. No quotes, markdown, explanation, or punctuation.",
    messages: [{ role: "user", content: prompt }],
    tools: [],
  })) {
    if (delta.type === "text") output += delta.text;
  }
  const title = sanitizeSessionTitle(output);
  return title ? title : null;
}
