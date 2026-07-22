/**
 * Minimal frontmatter parser (no YAML dependency).
 * Supports simple key: value lines and JSON-ish arrays for globs.
 * Inspired by oh-my-pi's frontmatter handling, simplified for console.
 */

export interface ParsedFrontmatter {
  frontmatter: Record<string, unknown>;
  body: string;
}

function kebabToCamel(key: string): string {
  if (!key.includes("-")) return key;
  return key.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
}

function parseScalar(raw: string): unknown {
  const value = raw.trim();
  if (!value) return "";
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return value.slice(1, -1);
  }
  if (value === "true") return true;
  if (value === "false") return false;
  if (value === "null") return null;
  if (/^-?\d+(\.\d+)?$/.test(value)) return Number(value);
  if (value.startsWith("[") && value.endsWith("]")) {
    try {
      return JSON.parse(value.replace(/'/g, '"'));
    } catch {
      return value
        .slice(1, -1)
        .split(",")
        .map((s) => s.trim().replace(/^["']|["']$/g, ""))
        .filter(Boolean);
    }
  }
  return value;
}

/**
 * Parse optional YAML-like frontmatter delimited by `---`.
 * Returns empty frontmatter when none is present.
 */
export function parseFrontmatter(content: string): ParsedFrontmatter {
  const text = content.replace(/^\uFEFF/, "");
  if (!text.startsWith("---")) {
    return { frontmatter: {}, body: text };
  }

  const end = text.indexOf("\n---", 3);
  if (end === -1) {
    return { frontmatter: {}, body: text };
  }

  const rawMeta = text.slice(3, end).replace(/^\r?\n/, "");
  let body = text.slice(end + 4);
  if (body.startsWith("\r\n")) body = body.slice(2);
  else if (body.startsWith("\n")) body = body.slice(1);

  const frontmatter: Record<string, unknown> = {};
  for (const line of rawMeta.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const colon = trimmed.indexOf(":");
    if (colon <= 0) continue;
    const key = kebabToCamel(trimmed.slice(0, colon).trim());
    const value = parseScalar(trimmed.slice(colon + 1));
    frontmatter[key] = value;
  }

  return { frontmatter, body };
}

export function asStringArray(value: unknown): string[] | undefined {
  if (typeof value === "string") {
    const parts = value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    return parts.length ? parts : undefined;
  }
  if (Array.isArray(value)) {
    const parts = value
      .filter((v): v is string => typeof v === "string")
      .map((s) => s.trim())
      .filter(Boolean);
    return parts.length ? parts : undefined;
  }
  return undefined;
}

export function asBoolean(value: unknown): boolean | undefined {
  if (typeof value === "boolean") return value;
  if (typeof value === "string") {
    const n = value.trim().toLowerCase();
    if (n === "true" || n === "1") return true;
    if (n === "false" || n === "0") return false;
  }
  return undefined;
}
