// The transcript's markdown rendering, on nsh's built-in `TextView`: the
// framework parses and lays out CommonMark + GFM itself, with built-in
// syntax highlighting for code blocks, task lists, tables, and a streaming
// mode. This file is the seam that keeps the desktop's transcript metrics
// (`Metrics`, from console-ui's `markdown/render.rs`): the scale constants
// live here and are passed to `TextView` as root/part styles.
//
// This replaces the hand-rolled block renderer (and, on the Rust side,
// removes the need for a `pulldown-cmark` bridge parser — the tree type in
// `./types.ts` remains documented as the bridge contract should the Rust
// core ever need to ship parsed blocks).

import { TextView, type TextViewStyles } from "nsh/base";

/// Type scales, from render.rs's `Metrics`.
export const METRICS = {
  BODY: { textSize: 12.5, lineHeight: 20, codeTextSize: 12.5, codeLineHeight: 18.5, blockGap: 10 },
  USER_MESSAGE: { textSize: 14, lineHeight: 20, codeTextSize: 12.5, codeLineHeight: 18.5, blockGap: 10 },
  COMPACT: { textSize: 11.5, lineHeight: 17, codeTextSize: 10.5, codeLineHeight: 16, blockGap: 6 },
} as const;
export type Metrics = (typeof METRICS)[keyof typeof METRICS];

/// Heading scale relative to body text, by level (`heading_metrics` in
/// render.rs), passed as the heading parts' styles.
function headingStyles(metrics: Metrics): Pick<TextViewStyles, "heading" | "heading1" | "heading2" | "heading3" | "heading4"> {
  const part = (level: number) => {
    const scale = level <= 1 ? 1.45 : level === 2 ? 1.28 : level <= 3 ? 1.14 : level === 4 ? 1.05 : 1.0;
    const size = Math.round(metrics.textSize * scale);
    return { fontSize: size, lineHeight: Math.round(size * 1.42) };
  };
  return {
    heading: { fontWeight: "600" },
    heading1: part(1),
    heading2: part(2),
    heading3: part(3),
    heading4: part(4),
  };
}

/// `TextView`'s part styles for a transcript scale: the desktop's body/code
/// sizes and line heights on the root (they pass to every part), then the
/// heading scale over it.
export function textViewStyles(metrics: Metrics): TextViewStyles {
  return {
    root: {
      fontSize: metrics.textSize,
      lineHeight: metrics.lineHeight,
    },
    ...headingStyles(metrics),
    codeBlockCode: {
      fontSize: metrics.codeTextSize,
      lineHeight: metrics.codeLineHeight,
    },
    code: { fontSize: metrics.textSize - 0.5 },
  };
}

/// A whole markdown document (or one message's slice of one), rendered at a
/// transcript scale. Pass `source` as it streams — `TextView` keeps its
/// blocks by position and only re-renders the ones that changed.
export function MarkdownBlocks(props: {
  /// The markdown source.
  source: string;
  metrics?: Metrics;
}) {
  return (
    <TextView
      class="w-full min-w-0"
      markdown={props.source}
      styles={textViewStyles(props.metrics ?? METRICS.BODY)}
    />
  );
}
