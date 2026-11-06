// Tests for the diff popover, beside `chat/diff_popover.tsx`, running
// through `diff_popover.fixture.tsx`.
import { expect, open, test } from "nsh/test";

test("a long diff previews its first 400 lines and offers the full diff", async () => {
  const app = await open("./diff_popover.fixture.tsx", { platform: "linux" });
  await app.press("label:Show diff for big.ts");
  expect(app.texts()).toContain("diff --git a/big.ts b/big.ts");
  expect(
    app.texts().some((t) => t.startsWith("Showing the first 400 of 431 lines")),
  ).toBe(true);
  await app.press("label:Open full diff");
  expect(app.texts()).toContain("Action: open:src/big.ts");
});

test("a binary file with no diff shows the binary note", async () => {
  const app = await open("./diff_popover.fixture.tsx", { platform: "linux" });
  await app.press("label:Show diff for logo.png");
  expect(app.texts()).toContain("Binary file added");
});
