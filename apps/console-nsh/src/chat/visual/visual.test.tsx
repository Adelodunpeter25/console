// Tests for the visual card, beside `visual/visual.tsx`. The runner opens
// `visual.fixture.tsx` (one card over every block kind) and presses through
// the fallback scenarios.
import { expect, open, test } from "nsh/test";

test("visual the card shows its title, metrics, table and chart summary", async () => {
  const app = await open("./visual.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Latency");
  expect(app.texts()).toContain("p50 by day");
  expect(app.texts()).toContain("p50");
  expect(app.texts()).toContain("128ms");
  expect(app.texts()).toContain("▼ -12%");
  expect(app.texts()).toContain("▲ +0.1%");
  expect(app.texts()).toContain("By region");
  expect(app.texts()).toContain("us-east");
  expect(app.texts()).toContain("12,400");
  // The chart frame's axis notes and one-line summary are visible text.
  expect(app.texts().some((t) => t.startsWith("x: day"))).toBe(true);
  expect(app.texts().some((t) => /^Line chart, p50 over days\./.test(t))).toBe(true);
});

test("visual an unsupported spec version shows the upgrade fallback", async () => {
  const app = await open("./visual.fixture.tsx", { platform: "linux" });
  await app.press("label:Show unsupported");
  expect(app.texts()).toContain("This visual needs a newer version of Console.");
});

test("visual a malformed spec shows the compact fallback, not a crash", async () => {
  const app = await open("./visual.fixture.tsx", { platform: "linux" });
  await app.press("label:Show invalid");
  expect(app.texts()).toContain("Visual unavailable");
});
