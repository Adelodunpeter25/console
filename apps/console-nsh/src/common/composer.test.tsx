// Tests for the composer, beside `common/composer.tsx`. The runner opens
// `composer.fixture.tsx`, which wires the Composer to real signals and
// renders the state it received on screen: the fixture runs in the app's
// engine, so the screen is how the tests observe it.
import { expect, open, test } from "nsh/test";
import {
  APPROVAL_MODES,
  approvalModeIcon,
  approvalModeLabel,
  thinkingLevelLabel,
} from "./composer";

test("composer renders its chips and placeholder", async () => {
  const app = await open("./composer.fixture.tsx");
  expect(app.texts()).toContain("Claude Opus");
  expect(app.texts()).toContain("Always Ask");
  expect(app.texts()).toContain("Thinking: medium");
  expect(app.texts()).toContain("Sent: none");
});

test("send is gated on a draft, and submit sends", async () => {
  const app = await open("./composer.fixture.tsx");
  // Empty draft: return is a no-op either way.
  await app.submit("id:composer-input");
  expect(app.texts()).toContain("Sent: none");
  await app.type("id:composer-input", "hello");
  await app.submit("id:composer-input");
  expect(app.texts()).toContain("Sent: hello");
});

test("the approval chip cycles the modes", async () => {
  const app = await open("./composer.fixture.tsx");
  const labels = APPROVAL_MODES.map(approvalModeLabel);
  for (let i = 0; i < APPROVAL_MODES.length; i++) {
    await app.press(`text:${labels[i]}`);
    const next = APPROVAL_MODES[(i + 1) % APPROVAL_MODES.length];
    expect(app.texts()).toContain(`Approval: ${next}`);
  }
});

test("thinking labels read the way the desktop's do", () => {
  expect(thinkingLevelLabel("None")).toBe("No thinking");
  expect(thinkingLevelLabel("High")).toBe("Thinking: high");
  expect(approvalModeIcon("FullAccess")).toBe("Zap");
  expect(approvalModeLabel("PlanMode")).toBe("Plan Mode");
});
