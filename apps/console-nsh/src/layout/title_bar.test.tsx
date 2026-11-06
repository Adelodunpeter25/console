// Tests for the window title bar, beside `layout/title_bar.tsx`. The runner
// opens `title_bar.fixture.tsx`; the fixture renders what it received on
// screen, so that is what the tests observe.
import { expect, open, test } from "nsh/test";

test("the bar shows the session's title and detail", async () => {
  const app = await open("./title_bar.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Fix the login loop");
  expect(app.texts()).toContain("— console");
});

test("the sidebar toggle reports presses", async () => {
  const app = await open("./title_bar.fixture.tsx", { platform: "linux" });
  await app.press("label:Toggle sidebar");
  expect(app.texts()).toContain("Pressed: sidebar");
});
