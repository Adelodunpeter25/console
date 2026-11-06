// Tests for the thinking block, beside `chat/thinking_block.tsx`.
import { expect, open, test } from "nsh/test";

test("the collapsed block shows the header and word count only", async () => {
  const app = await open("./thinking_block.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("THINKING");
  expect(app.texts()).toContain("(8 words)");
  expect(app.texts()).toContain("Collapsed: yes");
  // The reasoning text is hidden while collapsed.
  expect(app.texts().some((t) => t.includes("trace the bug"))).toBe(false);
});

test("pressing the header expands and re-collapses the block", async () => {
  const app = await open("./thinking_block.fixture.tsx", { platform: "linux" });
  await app.press("label:thinking-header-t1");
  expect(app.texts()).toContain("Collapsed: no");
  expect(app.texts().some((t) => t.includes("trace the bug"))).toBe(true);
  await app.press("label:thinking-header-t1");
  expect(app.texts()).toContain("Collapsed: yes");
  expect(app.texts().some((t) => t.includes("trace the bug"))).toBe(false);
});
