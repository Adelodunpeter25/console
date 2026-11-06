// Wire types for the v1 visual spec (`visualize` tool results), mirroring
// console-core's `types/visual.rs`. The Rust side serializes them; these are
// the decoded values (`parseVisual` below).

export interface VisualArtifact {
  version: number;
  spec: unknown;
}

export interface VisualPoint {
  x: number;
  y: number;
  label?: string | null;
}

export interface VisualSeries {
  name: string;
  points: VisualPoint[];
}

export interface VisualChart {
  kind: string;
  title?: string | null;
  xLabel?: string | null;
  yLabel?: string | null;
  series: VisualSeries[];
}

export type VisualBlock =
  | { type: "text"; text: string }
  | {
      type: "metric";
      label: string;
      value: string;
      delta?: string | null;
      trend?: string | null;
      caption?: string | null;
    }
  | {
      type: "table";
      caption?: string | null;
      columns: string[];
      rows: string[][];
    }
  | { type: "chart"; chart: VisualChart };

export interface VisualSpec {
  version: number;
  title?: string | null;
  subtitle?: string | null;
  blocks: VisualBlock[];
}

export type VisualParse =
  | { tag: "ok"; spec: VisualSpec }
  /** A newer spec version than this build understands. */
  | { tag: "unsupported" }
  | { tag: "invalid" };

export const SUPPORTED_VISUAL_VERSION = 1;

/// The version is checked before decoding so a newer spec never half-parses.
export function parseVisual(artifact: VisualArtifact | null | undefined): VisualParse {
  if (!artifact || artifact.version !== SUPPORTED_VISUAL_VERSION) {
    return { tag: "unsupported" };
  }
  const spec = decodeSpec(artifact.spec);
  if (!spec) return { tag: "invalid" };
  if (spec.version !== SUPPORTED_VISUAL_VERSION) return { tag: "unsupported" };
  return { tag: "ok", spec };
}

function decodeSpec(value: unknown): VisualSpec | null {
  if (typeof value !== "object" || value === null) return null;
  const v = value as Record<string, unknown>;
  if (typeof v.version !== "number" || !Array.isArray(v.blocks)) return null;
  const blocks: VisualBlock[] = [];
  for (const raw of v.blocks) {
    const block = decodeBlock(raw);
    if (!block) return null;
    blocks.push(block);
  }
  return {
    version: v.version,
    title: asText(v.title),
    subtitle: asText(v.subtitle),
    blocks,
  };
}

function decodeBlock(raw: unknown): VisualBlock | null {
  if (typeof raw !== "object" || raw === null) return null;
  const b = raw as Record<string, unknown>;
  switch (b.type) {
    case "text":
      return typeof b.text === "string" ? { type: "text", text: b.text } : null;
    case "metric":
      if (typeof b.label !== "string" || typeof b.value !== "string") return null;
      return {
        type: "metric",
        label: b.label,
        value: b.value,
        delta: asText(b.delta),
        trend: asText(b.trend),
        caption: asText(b.caption),
      };
    case "table":
      if (!Array.isArray(b.columns) || !b.columns.every((c) => typeof c === "string")) {
        return null;
      }
      if (!Array.isArray(b.rows) || !b.rows.every((r) => isStringArray(r))) return null;
      return {
        type: "table",
        caption: asText(b.caption),
        columns: b.columns as string[],
        rows: b.rows as string[][],
      };
    case "chart": {
      // The chart's fields are inline in the block (`{"type":"chart",
      // kind, series}`), like serde's internally-tagged newtype variant.
      const chart = decodeChart(b);
      return chart ? { type: "chart", chart } : null;
    }
    default:
      return null;
  }
}

function decodeChart(raw: unknown): VisualChart | null {
  if (typeof raw !== "object" || raw === null) return null;
  const c = raw as Record<string, unknown>;
  if (typeof c.kind !== "string" || !Array.isArray(c.series)) return null;
  const series: VisualSeries[] = [];
  for (const rawSeries of c.series) {
    if (typeof rawSeries !== "object" || rawSeries === null) return null;
    const s = rawSeries as Record<string, unknown>;
    if (typeof s.name !== "string" || !Array.isArray(s.points)) return null;
    const points: VisualPoint[] = [];
    for (const rawPoint of s.points) {
      if (typeof rawPoint !== "object" || rawPoint === null) return null;
      const p = rawPoint as Record<string, unknown>;
      if (typeof p.x !== "number" || typeof p.y !== "number" || Number.isNaN(p.x) || Number.isNaN(p.y)) {
        return null;
      }
      points.push({ x: p.x, y: p.y, label: asText(p.label) });
    }
    series.push({ name: s.name, points });
  }
  return { kind: c.kind, title: asText(c.title), xLabel: asText(c.xLabel), yLabel: asText(c.yLabel), series };
}

function asText(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((v) => typeof v === "string");
}
