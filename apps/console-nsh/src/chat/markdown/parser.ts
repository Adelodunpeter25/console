// The stub stand-in for the Rust markdown parser.
//
// The real parser is `pulldown-cmark` on the Rust side (`native/`), which
// ships a `BlockTree` (see `./types.ts`) over the bridge; this TS function is
// the seam the chat components render against until that pass lands. It
// covers only the shapes the transcript stubs need to look right — flat
// paragraphs, fenced code blocks, headings, rules — by a line scan, with no
// inline styling (runs come out plain). Nothing here promises CommonMark
// behaviour; replace, do not extend.

import type { Block, BlockTree, InlineRun, TopBlock } from "./types";

function plainRuns(text: string): InlineRun[] {
  return [{ text }];
}

/// Parse a source into a `BlockTree` — stub quality: line-based blocks only.
export function parseMarkdown(source: string): BlockTree {
  const blocks: TopBlock[] = [];
  const lines = source.split("\n");
  let index = 0;
  let offset = 0;

  while (index < lines.length) {
    const line = lines[index];
    const start = offset;

    // Fenced code block: ``` (with optional language) through the closing fence.
    const fence = /^```(\w*)\s*$/.exec(line);
    if (fence) {
      const language = fence[1] || null;
      index += 1;
      offset += line.length + 1;
      const codeLines: string[] = [];
      while (index < lines.length && !/^```\s*$/.test(lines[index])) {
        codeLines.push(lines[index]);
        offset += lines[index].length + 1;
        index += 1;
      }
      if (index < lines.length) {
        offset += lines[index].length + 1;
        index += 1;
      }
      blocks.push({
        range: [start, offset],
        block: { kind: "codeBlock", language, code: codeLines.join("\n") },
      });
      continue;
    }

    // Heading: `#` through `######`.
    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      const level = heading[1].length;
      const text = heading[2];
      index += 1;
      offset += line.length + 1;
      blocks.push({
        range: [start, offset],
        block: { kind: "heading", level, runs: plainRuns(text) },
      });
      continue;
    }

    // Thematic rule.
    if (/^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      index += 1;
      offset += line.length + 1;
      blocks.push({ range: [start, offset], block: { kind: "rule" } });
      continue;
    }

    // Blank line: ends any open paragraph.
    if (line.trim() === "") {
      index += 1;
      offset += line.length + 1;
      continue;
    }

    // Otherwise a paragraph runs to the next blank line or fence.
    const paragraphLines: string[] = [line];
    index += 1;
    offset += line.length + 1;
    while (
      index < lines.length &&
      lines[index].trim() !== "" &&
      !/^```/.test(lines[index]) &&
      !/^(#{1,6})\s/.test(lines[index])
    ) {
      paragraphLines.push(lines[index]);
      offset += lines[index].length + 1;
      index += 1;
    }
    const end = Math.min(offset, source.length);
    blocks.push({
      range: [start, end],
      block: { kind: "paragraph", runs: plainRuns(paragraphLines.join(" ")) },
    });
  }

  return { blocks };
}

/// The blocks of a tree, without their ranges.
export function blocksOf(tree: BlockTree): Block[] {
  return tree.blocks.map((top) => top.block);
}
