// Tests for the sidebar's project sections, beside
// `layout/sidebar/project_groups.tsx`. The runner opens
// `project_groups.fixture.tsx`; the fixture renders what it received on
// screen, so that is what the tests observe.
import { expect, open, test } from "nsh/test";

test("one section per project, newest first, no project last", async () => {
  const app = await open("./project_groups.fixture.tsx", {
    platform: "linux",
  });
  for (const label of ["console", "nsh", "No Project"]) {
    expect(app.texts()).toContain(label);
  }
  // Each section shows its chats; the no-project row keeps the fallback name.
  expect(app.texts()).toContain("Fix the login loop");
  expect(app.texts()).toContain("Ship the sidebar");
  expect(app.texts()).toContain("General");
});

test("a section collapses and re-expands over its chats", async () => {
  const app = await open("./project_groups.fixture.tsx", {
    platform: "linux",
  });
  await app.press("text:console >> nth=0");
  expect(app.texts().some((t) => t === "Fix the login loop")).toBe(false);
  // The other sections stay open.
  expect(app.texts()).toContain("Ship the sidebar");
  await app.press("text:console >> nth=0");
  expect(app.texts()).toContain("Fix the login loop");
});

test("a chat row opens its session", async () => {
  const app = await open("./project_groups.fixture.tsx", {
    platform: "linux",
  });
  await app.press("text:Ship the sidebar");
  expect(app.texts()).toContain("Pressed: s2");
});
