// Shared state for the composer fixture, so the fixture renders what the
// Composer received and did with it.
import { signal } from "nsh";
import type { ApprovalMode, ThinkingLevel } from "./composer";

export const draft = signal("");
export const sent = signal<string | null>(null);
export const approval = signal<ApprovalMode>("AlwaysAsk");
export const thinking = signal<ThinkingLevel>("Medium");
