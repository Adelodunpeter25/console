// Tests for the composer's status pills row, beside
// `common/composer_pills.tsx`. The runner opens `composer_pills.fixture.tsx`;
// the fixture renders the callbacks it received on screen.
import { expect, open, test } from "nsh/test";

test("the pills row shows each pill's summary", async () => {
  const app = await open("./composer_pills.fixture.tsx");
  expect(app.texts()).toContain("3 files changed");
  expect(app.texts()).toContain("+12");
  expect(app.texts()).toContain("-4");
  expect(app.texts()).toContain("1/3 tasks");
  expect(app.texts()).toContain("1 subagent");
  expect(app.texts()).toContain("1 shell");
});

test("the tasks pill opens its popover", async () => {
  const app = await open("./composer_pills.fixture.tsx");
  await app.press("label:Tasks");
  expect(app.texts()).toContain("TASKS");
  expect(app.texts()).toContain("Write the failing test");
  expect(app.texts()).toContain("Make it pass");
});

test("the subagents pill opens its popover", async () => {
  const app = await open("./composer_pills.fixture.tsx");
  await app.press("label:Subagents");
  expect(app.texts()).toContain("SUBAGENTS");
  // The role is the human-readable name; the raw name and id stay hidden.
  expect(app.texts()).toContain("Plan checker");
  expect(app.texts()).not.toContain("plan-checker");
});

test("the shells pill opens its popover and stops a shell", async () => {
  const app = await open("./composer_pills.fixture.tsx");
  await app.press("label:Shells");
  expect(app.texts()).toContain("SHELLS");
  expect(app.texts()).toContain("make dev-server");
  // The stop button stops that shell.
  await app.press("label:Stop make dev-server");
  expect(app.texts()).toContain("Stopped: sh1");
});

test("the changes pill opens the changes review", async () => {
  const app = await open("./composer_pills.fixture.tsx");
  await app.press("label:Open changes review");
  expect(app.texts()).toContain("Changes opened: yes");
});
