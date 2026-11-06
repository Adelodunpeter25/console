// Tests for the copy button, beside `common/copy_button.tsx`. The runner
// opens `copy_button.fixture.tsx`, which renders the copy callback's result
// on screen.
import { expect, open, test } from "nsh/test";

test("pressing the copy button copies and reports the copy", async () => {
  const app = await open("./copy_button.fixture.tsx");
  expect(app.texts()).toContain("Copied: none");
  await app.press("label:Copy");
  expect(app.texts()).toContain("Copied: copy me");
});
