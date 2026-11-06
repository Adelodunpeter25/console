// Shared state for the usage panel fixture.
import { signal } from "nsh";
import type { ContextSnapshot, UsageReport } from "./usage_meter";

export const report = signal<UsageReport | null>(null);
export const snapshot = signal<ContextSnapshot | null>(null);
export const loading = signal(false);
