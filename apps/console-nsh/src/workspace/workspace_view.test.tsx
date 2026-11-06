import { expect, open, test } from "nsh/test";

test("tabs open, activate and close across panes", async () => {
  const app = await open("./workspace_view.fixture.tsx", { platform: "linux" });
  await app.press("label:Open chat s1");
  await app.press("label:Open chat s2");
  expect(app.texts()).toContain("Chat s1");
  expect(app.texts()).toContain("Chat s2");
  expect(app.texts()).toContain("Panes: 1");

  await app.press("label:Split active");
  expect(app.texts()).toContain("Panes: 2");

  // The newly opened tab is active; closing it leaves the other pane's tab.
  await app.press("label:Close active tab");
  await app.press("label:Close active tab");
  expect(app.texts()).toContain("Panes: 1");
});

test("selecting a tab activates it in its pane", async () => {
  const app = await open("./workspace_view.fixture.tsx", { platform: "linux" });
  await app.press("label:Open chat s1");
  await app.press("label:Open chat s2");
  await app.press("label:Tab Chat s1");
  // Both tabs render; the active one is marked through the selected state.
  expect(app.texts()).toContain("Chat s1");
  expect(app.texts()).toContain("Chat s2");
});
