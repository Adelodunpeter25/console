// Tests for the queued prompt card, beside `common/queued_prompt_card.tsx`.
// The preview formatter is asserted directly; the actions run through the
// stack in `queued_prompt_card.fixture.tsx`.
import { expect, open, test } from "nsh/test";
import { formatPromptPreview } from "./queued_prompt_card";

test("the preview takes the first line and appends an ellipsis", () => {
  expect(formatPromptPreview("  first\n\nsecond  ")).toBe("first...");
  expect(formatPromptPreview("single line")).toBe("single line");
  expect(formatPromptPreview("   ")).toBe("");
});

test("the preview truncates long lines: 70 multi-line, 90 single-line", () => {
  const long = "a".repeat(120);
  expect(formatPromptPreview(`${long}\nsecond`)).toBe(
    `${"a".repeat(70)}...`,
  );
  expect(formatPromptPreview(long)).toBe(`${"a".repeat(90)}...`);
});

test("the stack previews each prompt and runs its actions", async () => {
  const app = await open("./queued_prompt_card.fixture.tsx");
  // Prompt 1: first line plus the annotation's component name (the preview's
  // trailing ellipsis can be visually clipped, so match the line itself).
  expect(app.texts()).toContain("Fix the login loop");
  expect(app.texts()).toContain("LoginForm");
  // Prompt 2: long single line truncated at 90 chars.
  expect(
    app.texts().some((text) => text.startsWith("This one has a very long") && text.endsWith("...")),
  ).toBe(true);

  await app.press("label:Edit queued prompt q1");
  expect(app.texts()).toContain("Action: edit:q1");
  await app.press("label:Delete queued prompt q1");
  expect(app.texts()).toContain("Action: delete:q1");
  await app.press("label:Send queued prompt q2 now");
  expect(app.texts()).toContain("Action: steer:q2");
});
