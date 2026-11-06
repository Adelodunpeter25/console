// Tests for the user message bubble, beside `message_user.tsx`. The fold
// goes through `message_user.fixture.tsx`; the fold estimate is a pure
// function asserted directly.
import { expect, open, test } from "nsh/test";
import { estimatedTextLines } from "./message_user";

test("estimatedTextLines counts hard breaks plus wrapping", () => {
  expect(estimatedTextLines("one line")).toBe(1);
  expect(estimatedTextLines("a\nb\nc")).toBe(3);
  expect(estimatedTextLines("x".repeat(129))).toBe(3);
});

test("the user bubble folds long prompts behind Show more", async () => {
  const app = await open("./message_user.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Show more");
  // Folded, only the first eight lines render.
  expect(
    app.texts().some((t) => t.includes("Line 9 of a long prompt")),
  ).toBe(false);
  await app.press("label:Toggle expand");
  expect(app.texts()).toContain("Show less");
  expect(
    app.texts().some((t) => t.includes("Line 12 of a long prompt")),
  ).toBe(true);
});

test("context files paint as mention pills inside the bubble", async () => {
  const app = await open("./message_user.fixture.tsx", { platform: "linux" });
  // Nested mention texts flatten into the paragraph string; the pill
  // segment is present within it.
  expect(app.texts().some((t) => t.includes("main.rs"))).toBe(true);
});
