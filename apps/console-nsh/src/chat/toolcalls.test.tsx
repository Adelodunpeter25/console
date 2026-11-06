// Tests for the grouped run-activity surface, beside
// `chat/toolcalls.tsx`. The runner opens `toolcalls.fixture.tsx`, which
// renders one run with a thinking step, a text step and three tool calls.
import { expect, open, test } from "nsh/test";

test("the finished run renders its summary and expands its timeline", async () => {
  const app = await open("./toolcalls.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Worked for 37s");
  await app.press("label:Run run-1");
  expect(app.texts()).toContain("On it — checking the build.");
  expect(app.texts()).toContain("Reading file");
  expect(app.texts()).toContain("src/main.rs");
  expect(app.texts()).toContain("Running shell");
  expect(app.texts()).toContain("cargo build");
});

test("a call row expands to its Arguments and Result sections", async () => {
  const app = await open("./toolcalls.fixture.tsx", { platform: "linux" });
  await app.press("label:Run run-1");
  await app.press("label:Tool call Running shell");
  expect(app.texts()).toContain("Arguments");
  expect(
    app.texts().some((text) => text.includes('"command": "cargo build"')),
  ).toBe(true);
  expect(app.texts()).toContain("Result");
  expect(app.texts()).toContain("Compilation finished");
});

test("an edit row shows its diff counts instead of arguments", async () => {
  const app = await open("./toolcalls.fixture.tsx", { platform: "linux" });
  await app.press("label:Run run-1");
  await app.press("label:Tool call Editing file");
  expect(app.texts()).toContain("+1");
  expect(app.texts()).toContain("-1");
  expect(app.texts()).not.toContain("Arguments");
});

test("the working run shows the live indicator and expands by default", async () => {
  const app = await open("./toolcalls.fixture.tsx", { platform: "linux" });
  await app.press("label:Toggle working");
  expect(app.texts().some((t) => /^Working for \d+s$/.test(t))).toBe(true);
  // An actively-working run is expanded without a press.
  expect(app.texts()).toContain("Reading file");
});
