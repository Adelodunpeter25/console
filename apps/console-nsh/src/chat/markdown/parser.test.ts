// Tests for the markdown tree contract and the stub parser, beside
// `chat/markdown/`. The tree shape is what the Rust side will ship; the stub
// parser's line-scan behaviour is pinned here until that pass lands.
import { expect, test } from "nsh/test";
import { parseMarkdown } from "./parser";
import { plain, run } from "./types";

test("the stub parses paragraphs, headings, rules and code blocks", () => {
  const tree = parseMarkdown(
    "First paragraph,\nstill the same one.\n\n## A heading\n\n```rust\nfn main() {}\n```\n\n---\n",
  );
  expect(tree.blocks.map((top) => top.block.kind)).toEqual([
    "paragraph",
    "heading",
    "codeBlock",
    "rule",
  ]);
  expect(tree.blocks[0].block).toEqual({
    kind: "paragraph",
    runs: [plain("First paragraph, still the same one.")],
  });
  expect(tree.blocks[1].block).toEqual({
    kind: "heading",
    level: 2,
    runs: [plain("A heading")],
  });
  expect(tree.blocks[2].block).toEqual({
    kind: "codeBlock",
    language: "rust",
    code: "fn main() {}",
  });
});

test("top-level blocks carry byte ranges that cover their source", () => {
  const source = "one\n\ntwo\n";
  const tree = parseMarkdown(source);
  expect(tree.blocks.length).toBe(2);
  for (const top of tree.blocks) {
    const [start, end] = top.range;
    expect(start).toBeLessThanOrEqual(end);
    expect(source.slice(start, end).trim().length).toBeGreaterThan(0);
  }
  // The last block's range end is the whole source's end.
  expect(tree.blocks[1].range[1]).toBe(source.length);
});

test("an unterminated fence still yields its code", () => {
  const tree = parseMarkdown("```\ncode without a close");
  expect(tree.blocks).toEqual([
    {
      range: tree.blocks[0].range,
      block: { kind: "codeBlock", language: null, code: "code without a close" },
    },
  ]);
});

test("the tree helpers build styled runs by hand", () => {
  expect(run("hi", { bold: true, link: "https://x" })).toEqual({
    text: "hi",
    style: { bold: true, link: "https://x" },
  });
  expect(plain("hi")).toEqual({ text: "hi" });
});
