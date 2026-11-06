// The markdown tree the Rust side will parse and the UI renders, ported from
// console-ui's `markdown/parser.rs` tree model (`InlineStyle`, `InlineRun`,
// `Block`, `TopBlock`, `BlockTree`). This is the contract between the two
// sides: the Rust crate owns `pulldown-cmark` parsing and ships this tree
// over the bridge; the TSX side only renders it.

/// Inline styling threaded through nested emphasis and links.
export interface InlineStyle {
  bold?: boolean;
  italic?: boolean;
  code?: boolean;
  strikethrough?: boolean;
  /// The run is LaTeX source (`$…$`), typeset by the renderer; the text is
  /// the formula, not display text.
  math?: boolean;
  /// Destination URL when inside a link.
  link?: string;
}

/// One run of identically styled inline text.
export interface InlineRun {
  text: string;
  style?: InlineStyle;
}

/// GFM column alignment. Unspecified renders as left.
export type TableAlign = "left" | "center" | "right";

/// A list item: its blocks plus an optional task-list checkbox state.
export interface ListItem {
  task?: boolean;
  blocks: Block[];
}

/// A markdown block. Containers nest.
export type Block =
  | { kind: "paragraph"; runs: InlineRun[] }
  /// A standalone image. Inline images split their paragraph so the text
  /// before and after keeps its order around them.
  | { kind: "image"; url: string; alt: string }
  /// `$$…$$`: a display formula, as LaTeX source.
  | { kind: "math"; latex: string }
  | { kind: "heading"; level: number; runs: InlineRun[] }
  | { kind: "codeBlock"; language: string | null; code: string }
  | { kind: "blockQuote"; children: Block[] }
  | {
      kind: "list";
      /// Defined for an ordered list, holding its first number.
      orderedStart?: number;
      items: ListItem[];
    }
  | {
      kind: "table";
      header: InlineRun[][];
      rows: InlineRun[][][];
      align: TableAlign[];
    }
  | { kind: "rule" };

/// A top-level block plus its byte range in the source. The range start of
/// the last block is the *stable boundary* the streaming parser exploits:
/// appending to the source cannot change anything before it.
export interface TopBlock {
  /// [start, end) byte offsets into the source.
  range: [number, number];
  block: Block;
}

/// Top-level blocks in document order.
export interface BlockTree {
  blocks: TopBlock[];
}

/// Helpers for building trees by hand (tests, fixtures, and until the Rust
/// parser lands, callers that assemble blocks themselves).

export function plain(text: string): InlineRun {
  return { text };
}

export function run(text: string, style: InlineStyle): InlineRun {
  return { text, style };
}
