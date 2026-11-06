// Tests for the tooltip bubble, beside `common/tooltip.tsx`. The runner
// opens `tooltip.fixture.tsx`; hovering the trigger shows the bubble after
// the framework's hover delay.
import { expect, open, test } from "nsh/test";

test("hovering the trigger shows the hint, leaving hides it", async () => {
  const app = await open("./tooltip.fixture.tsx", { platform: "linux" });
  expect(app.texts()).not.toContain("Sort by project");
  await app.hover("label:Sort mode");
  await app.wait(700);
  expect(app.texts()).toContain("Sort by project");
  await app.hover("text:elsewhere");
  await app.wait(300);
  expect(app.texts()).not.toContain("Sort by project");
});
