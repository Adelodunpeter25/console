// Tests for the error banner, beside `common/error_banner.tsx`. The runner
// opens `error_banner.fixture.tsx`, which renders the dismiss callback's
// result on screen.
import { expect, open, test } from "nsh/test";

test("the banner shows its message and dismisses on copy", async () => {
  const app = await open("./error_banner.fixture.tsx");
  expect(app.texts()).toContain("Session failed to start: connection refused");
  expect(app.texts()).toContain("Dismissed: no");
  // Copying the message dismisses the banner, as on the desktop.
  await app.press("label:Copy");
  expect(app.texts()).toContain("Dismissed: yes");
});
