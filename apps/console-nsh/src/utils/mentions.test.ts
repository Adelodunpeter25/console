// Tests for the mention/comment/paste text helpers, beside
// `utils/mentions.ts`. These are the console-shared subsets the bubbles and
// composer build on.
import { expect, test } from "nsh/test";
import {
  findPasteLabel,
  messageTextWithMentions,
  parseChatRefs,
  splitFileComments,
} from "./mentions";

test("message text keeps one flat string and paints mention ranges", () => {
  const [text, mentions] = messageTextWithMentions(
    "look at src/main.rs and then README.md",
    ["src/main.rs", "README.md"],
  );
  // A matched path paints as its friendly basename pill.
  expect(text).toBe("look at main.rs and then README.md");
  expect(mentions.map((m) => m.path)).toEqual(["src/main.rs", "README.md"]);
});

test("full paths win over bare filenames at the same position", () => {
  const [text, mentions] = messageTextWithMentions("src/main.rs", [
    "src/main.rs",
    "main.rs",
  ]);
  expect(text).toBe("main.rs");
  expect(mentions.length).toBe(1);
  expect(mentions[0].path).toBe("src/main.rs");
});

test("pasted-N-lines labels map to their paste paths in order", () => {
  const [text, mentions] = messageTextWithMentions(
    "pasted 12 lines\nthen pasted 3 lines more",
    ["/srv/pastes/s1/pasted-1.log", "/srv/pastes/s1/pasted-2.log"],
  );
  expect(text).toBe("pasted 12 lines\nthen pasted 3 lines more");
  expect(mentions.map((m) => m.path)).toEqual([
    "/srv/pastes/s1/pasted-1.log",
    "/srv/pastes/s1/pasted-2.log",
  ]);
});

test("findPasteLabel bounds the friendly large-paste text", () => {
  expect(findPasteLabel("pasted 4 lines rest")).toEqual([0, 14]);
  // "pasted 0 lines" — no digits, no match; "pasted 1 line" is not lines.
  expect(findPasteLabel("pasted lines")).toBeUndefined();
  expect(findPasteLabel("end pasted 1,000 lines")).toEqual([4, 22]);
});

test("chat refs without metadata are found by the shared $ref rules", () => {
  const [text, mentions] = messageTextWithMentions("compare $alpha and $beta", []);
  expect(text).toBe("compare $alpha and $beta");
  expect(mentions.map((m) => m.path)).toEqual(["chat:alpha", "chat:beta"]);
  // Prices stay plain text: a digit-led token needs id shape.
  const [, prices] = messageTextWithMentions("it cost $5 total", []);
  expect(prices).toEqual([]);
});

test("parseChatRefs trims edge punctuation and rejects prices", () => {
  expect(parseChatRefs("$alpha-").length).toBe(1);
  expect(parseChatRefs("a$b").length).toBe(0);
});

test("splitFileComments strips tagged blocks into chips", () => {
  const content =
    'fix this <file_comment path="src/a.rs" lines="1–2">\nnote\n</file_comment>';
  const [text, comments] = splitFileComments(content);
  expect(text).toBe("fix this");
  expect(comments.length).toBe(1);
  expect(comments[0].path).toBe("src/a.rs");
  expect(comments[0].lines).toBe("1–2");
  expect(comments[0].text).toBe("note");
});
