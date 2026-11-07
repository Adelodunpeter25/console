// Tests for the diff popover, beside `chat/diff_popover.tsx`, running
// through `diff_popover.fixture.tsx`. The card is a `HoverCard`: on a
// desktop host it opens on pointer rest, which a headless press cannot do,
// so the tests open on the touch host (`android`), where pressing the
// trigger opens and closes the card.
import { expect, open, test } from "nsh/test";

test("a long diff previews its first 200 lines and offers the full diff", async () => {
  const app = await open("./diff_popover.fixture.tsx", { platform: "android" });
  await app.press("label:Show diff for big.ts");
  // The card opens asynchronously after the press; give it a beat so the
  // read below doesn't race the render.
  await new Promise((r) => setTimeout(r, 200));
  // The DiffView renders parsed diff lines (the +/- itself is a change bar,
  // not text); the `diff --git` file header is not part of the line view.
  expect(app.texts()).toContain("line 1");
  expect(
    app.texts().some((t) => t.startsWith("Showing the first 200 of 430 lines")),
  ).toBe(true);
  await app.press("label:Open full diff");
  expect(app.texts()).toContain("Action: open:src/big.ts");
});

test("a binary file with no diff shows the binary note", async () => {
  const app = await open("./diff_popover.fixture.tsx", { platform: "android" });
  await app.press("label:Show diff for logo.png");
  expect(app.texts()).toContain("Binary file added");
});
