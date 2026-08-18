import { z } from "zod";

export interface ToolInputValidation {
  success: boolean;
  data?: unknown;
  repairedPaths: string[];
  issues?: z.ZodIssue[];
}

const PATH_ALIASES: Record<string, string> = {
  filePath: "path",
  target_file: "path",
  absolutePath: "path",
};

const MARKDOWN_LINK = /^\[([^\]]+)\]\((?:https?:\/\/)?[^)]+\)$/;

function cloneInput(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(cloneInput);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, child]) => [key, cloneInput(child)]),
    );
  }
  return value;
}

function getAtPath(value: unknown, issuePath: (string | number)[]): unknown {
  let current = value;
  for (const segment of issuePath) {
    if (current === null || current === undefined) return undefined;
    current = (current as Record<string | number, unknown>)[segment];
  }
  return current;
}

function setAtPath(root: unknown, issuePath: (string | number)[], value: unknown): boolean {
  if (issuePath.length === 0 || root === null || typeof root !== "object") return false;
  let current = root as Record<string | number, unknown>;
  for (const segment of issuePath.slice(0, -1)) {
    const next = current[segment];
    if (next === null || typeof next !== "object") return false;
    current = next as Record<string | number, unknown>;
  }
  current[issuePath[issuePath.length - 1]!] = value;
  return true;
}

function deleteAtPath(root: unknown, issuePath: (string | number)[]): boolean {
  if (issuePath.length === 0 || root === null || typeof root !== "object") return false;
  let current = root as Record<string | number, unknown>;
  for (const segment of issuePath.slice(0, -1)) {
    const next = current[segment];
    if (next === null || typeof next !== "object") return false;
    current = next as Record<string | number, unknown>;
  }
  const leaf = issuePath[issuePath.length - 1]!;
  if (!(leaf in current)) return false;
  delete current[leaf];
  return true;
}

function repairAliases(input: unknown): string[] {
  if (!input || typeof input !== "object" || Array.isArray(input)) return [];
  const record = input as Record<string, unknown>;
  const repaired: string[] = [];
  for (const [alias, canonical] of Object.entries(PATH_ALIASES)) {
    if (!(canonical in record) && alias in record) {
      record[canonical] = record[alias];
      delete record[alias];
      repaired.push(canonical);
    }
  }
  return repaired;
}

/**
 * Repair only recoverable model-shaped input after the raw schema parse fails.
 * A successful raw parse is returned untouched so valid content is never
 * rewritten merely because it resembles JSON or a markdown link.
 */
export function validateToolInput(schema: z.ZodTypeAny, raw: unknown): ToolInputValidation {
  const direct = schema.safeParse(raw);
  if (direct.success) return { success: true, data: direct.data, repairedPaths: [] };

  const candidate = cloneInput(raw);
  const repairedPaths = repairAliases(candidate);
  let issues = direct.error.issues;

  // Iterate a small bounded number of times because one repair can expose a
  // nested issue (for example, a stringified array containing malformed items).
  for (let pass = 0; pass < 3; pass++) {
    let changed = false;

    for (const issue of issues) {
      const value = getAtPath(candidate, issue.path);

      // Optional fields commonly arrive as null. Removing the field is safe:
      // required fields still fail on the re-parse and are not silently filled.
      if (issue.code === z.ZodIssueCode.invalid_type && issue.received === "null") {
        const didRepair = deleteAtPath(candidate, issue.path);
        changed = didRepair || changed;
        if (didRepair) repairedPaths.push(issue.path.join("."));
        continue;
      }

      if (
        issue.code === z.ZodIssueCode.invalid_type &&
        issue.expected === "array" &&
        typeof value === "string"
      ) {
        // Parse stringified arrays before treating the value as a bare string.
        try {
          const parsed = JSON.parse(value) as unknown;
          if (Array.isArray(parsed)) {
            const didRepair = setAtPath(candidate, issue.path, parsed);
            changed = didRepair || changed;
            if (didRepair) repairedPaths.push(issue.path.join("."));
            continue;
          }
        } catch {
          // The value may simply be a bare string.
        }
        const didRepair = setAtPath(candidate, issue.path, [value]);
        changed = didRepair || changed;
        if (didRepair) repairedPaths.push(issue.path.join("."));
        continue;
      }

      if (
        issue.code === z.ZodIssueCode.invalid_type &&
        issue.expected === "array" &&
        value !== null &&
        typeof value === "object" &&
        !Array.isArray(value)
      ) {
        const didRepair = setAtPath(candidate, issue.path, [value]);
        changed = didRepair || changed;
        if (didRepair) repairedPaths.push(issue.path.join("."));
        continue;
      }

      // Path-aware schemas reject chat-style markdown links. Unwrap only when
      // validation has already failed; ordinary valid strings are untouched.
      if (typeof value === "string") {
        const match = MARKDOWN_LINK.exec(value);
        if (match) {
          const didRepair = setAtPath(candidate, issue.path, match[1]);
          changed = didRepair || changed;
          if (didRepair) repairedPaths.push(issue.path.join("."));
        }
      }
    }

    if (!changed) break;
    const reparsed = schema.safeParse(candidate);
    if (reparsed.success) {
      return { success: true, data: reparsed.data, repairedPaths };
    }
    issues = reparsed.error.issues;
  }

  return { success: false, repairedPaths, issues };
}

/** Reject chat-style markdown links in values that will be passed to fopen/fs APIs. */
export function pathString(description: string) {
  return z
    .string()
    .refine((value) => !MARKDOWN_LINK.test(value), {
      message: "must be a plain filesystem path, not a markdown link",
    })
    .describe(description);
}
