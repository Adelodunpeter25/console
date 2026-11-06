// Tests for the sidebar's draft rows, beside
// `layout/sidebar/draft_item.tsx`. The runner opens
// `draft_item.fixture.tsx`; the fixture renders what it received on screen,
// so that is what the tests observe.
import { expect, open, test } from "nsh/test";
import { formatTimeAgo } from "../../utils/time";

test("a draft shows its title, preview and time ago", async () => {
  const app = await open("./draft_item.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Half-written prompt");
  expect(app.texts()).toContain("refactor the com");
  expect(app.texts()).toContain(formatTimeAgo(1_700_000_000_000, 1_700_000_000_000));
});

test("a draft of a session opens it; the new-chat draft starts one", async () => {
  const app = await open("./draft_item.fixture.tsx", { platform: "linux" });
  await app.press("text:Half-written prompt");
  expect(app.texts()).toContain("Pressed: s1");
  await app.press("text:New chat");
  expect(app.texts()).toContain("Pressed: new");
});

test("the context menu discards the draft", async () => {
  const app = await open("./draft_item.fixture.tsx", { platform: "linux" });
  await app.rightClick("text:Half-written prompt");
  // Headless, the ContextMenu renders in place rather than as a native menu.
  await app.press("text:Delete Draft");
  expect(app.texts()).toContain("Deleted: s1");
});
