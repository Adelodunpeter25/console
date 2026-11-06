// Tests for the composer, beside `common/composer.tsx`. The runner opens
// `composer.fixture.tsx`, which wires the Composer to real signals and
// renders the state it received on screen: the fixture runs in the app's
// engine, so the screen is how the tests observe it.
import { expect, open, test } from "nsh/test";
import { approvalModeLabel } from "./approval_selector";

test("composer renders its chips and placeholder", async () => {
  const app = await open("./composer.fixture.tsx");
  // The model chip shows the selected model's display name.
  expect(app.texts()).toContain("Claude Opus 5");
  // The approval chip shows the desktop's label ("Normal" for AlwaysAsk).
  expect(app.texts()).toContain("Normal");
  expect(app.texts()).toContain("Thinking: medium");
  expect(app.texts()).toContain("Sent: none");
});

test("send is gated on a draft, and submit sends", async () => {
  const app = await open("./composer.fixture.tsx");
  // Empty draft: submit is a no-op either way.
  await app.submit("id:composer-input");
  expect(app.texts()).toContain("Sent: none");
  await app.type("id:composer-input", "hello");
  await app.submit("id:composer-input");
  expect(app.texts()).toContain("Sent: hello");
});

test("the approval chip opens the mode dropdown and picks a mode", async () => {
  const app = await open("./composer.fixture.tsx");
  await app.press("label:Approval mode");
  // The dropdown card lists every mode with its description.
  expect(app.texts()).toContain("Read-only mode, no file edits or command execution");
  await app.press(`text:${approvalModeLabel("PlanMode")}`);
  expect(app.texts()).toContain("Approval: PlanMode");
});

test("the model chip opens the picker: search, tabs and favourites", async () => {
  const app = await open("./composer.fixture.tsx");
  await app.press("label:Select model");
  // The provider tabs are the catalog's; the first provider is active.
  expect(app.texts()).toContain("Claude Opus 5");
  expect(app.texts()).toContain("Anthropic · 200k context");
  // The filter narrows the active provider's list, empty state included.
  await app.type("id:model-search", "gpt");
  expect(app.texts()).toContain("No models found");
  await app.clear("id:model-search");
  // The favourites tab lists the fixture's starred model.
  await app.press("label:Favourites");
  expect(app.texts()).toContain("Claude Sonnet 5");
  expect(app.texts()).not.toContain("No Favourites");
  // Switching providers, a press picks the model.
  await app.press("label:OpenAI");
  await app.press("text:Gpt 6");
  expect(app.texts()).toContain("Selected: openai:gpt-6");
});
