// Tests for the composer autocomplete helpers, beside
// `common/composer_autocomplete.ts`: trigger detection boundaries, filtering
// and caps, and the inserted text per kind.
import { expect, test } from "nsh/test";
import {
  detectTrigger,
  filterItems,
  itemInsertText,
  slugifyChatTitle,
} from "./composer_autocomplete";

const commands = [
  { name: "plan", description: "Plan before acting" },
  { name: "review", description: "Review the current changes" },
];
const files = [
  { relativePath: "src/main.rs", absolutePath: "/repo/src/main.rs" },
  { relativePath: "docs/overview.md", absolutePath: "/repo/docs/overview.md" },
];
const chats = [
  { id: "s1", title: "Fix the login loop" },
  { id: "s2", title: "Review plan feedback" },
];

test("detectTrigger finds slash commands at a boundary only", () => {
  expect(detectTrigger("/pl", 3)).toEqual({
    kind: "command",
    range: { start: 0, end: 3 },
    query: "pl",
  });
  // Mid-word slash is prose, not a trigger.
  expect(detectTrigger("and/or", 6)).toBeUndefined();
  // After whitespace anywhere in the prompt.
  const mid = detectTrigger("hey /rev", 8);
  expect(mid?.kind).toBe("command");
  expect(mid?.query).toBe("rev");
});

test("detectTrigger finds @ file and $ chat refs with boundary rules", () => {
  const at = detectTrigger("look at @src/ma", 15);
  expect(at?.kind).toBe("file");
  expect(at?.query).toBe("src/ma");
  // An email address is not a file ref.
  expect(detectTrigger("mail me a@b", 11)).toBeUndefined();
  const dollar = detectTrigger("see $fix-th", 11);
  expect(dollar?.kind).toBe("chat");
  expect(dollar?.query).toBe("fix-th");
  // A digit-only $ query is a price, not a ref.
  expect(detectTrigger("it costs $12", 12)).toBeUndefined();
});

test("filterItems ranks and caps like the desktop", () => {
  const command = filterItems(commands, files, chats, {
    kind: "command",
    range: { start: 0, end: 5 },
    query: "pl",
  });
  expect(command.map((item) => itemInsertText(item))).toEqual(["/plan "]);
  // Empty file query lists the first files; a path substring matches.
  const file = filterItems(commands, files, chats, {
    kind: "file",
    range: { start: 0, end: 8 },
    query: "overview",
  });
  expect(file.map((item) => itemInsertText(item))).toEqual(["overview.md "]);
  // Chats: slug prefix outranks a contains, ties break by title.
  const chat = filterItems(commands, files, chats, {
    kind: "chat",
    range: { start: 0, end: 4 },
    query: "fix",
  });
  expect(chat).toHaveLength(1);
  expect(itemInsertText(chat[0])).toBe("$fix-the-login-loop ");
});

test("slugifyChatTitle matches the Rust slug", () => {
  expect(slugifyChatTitle("Fix the Login Loop!")).toBe("fix-the-login-loop");
  expect(slugifyChatTitle("  spaced---out  ")).toBe("spaced-out");
});
