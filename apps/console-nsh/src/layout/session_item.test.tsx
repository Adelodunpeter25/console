// Tests for the sidebar's chat card, beside `layout/session_item.tsx`.
// The runner opens `session_item.fixture.tsx`; the fixture renders the
// callbacks it received on screen, so that is what the tests observe.
import { expect, open, test } from "nsh/test";
import { formatTimeAgo } from "../utils/time";

test("the card shows its rows, the General fallback and the draft marker", async () => {
  const app = await open("./session_item.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Fix the login loop");
  // No project: the desktop's fallback folder name.
  expect(app.texts()).toContain("General");
  expect(app.texts()).toContain("Draft");
  expect(app.texts()).toContain(formatTimeAgo(1_700_000_000_000, 1_700_000_000_000));
});

test("the settle hover button settles and opens the chat", async () => {
  const app = await open("./session_item.fixture.tsx", { platform: "linux" });
  // The button hides until the pointer rests on the card.
  await app.hover("text:Fix the login loop");
  await app.press("label:Settle chat");
  expect(app.texts()).toContain("Toggled: settled");
  await app.press("text:Fix the login loop");
  expect(app.texts()).toContain("Pressed: opened");
});
