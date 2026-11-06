// Tests for the interaction cards, beside `chat/interaction_card.tsx`. The
// runner opens `interaction_card.fixture.tsx`, which renders a permission
// card and a question card over local signals.
import { expect, open, test } from "nsh/test";

test("the permission card shows the tool and runs allow / deny", async () => {
  const app = await open("./interaction_card.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Permission required: editFile");
  expect(app.texts()).toContain("Needs to edit a file");

  await app.press("label:Deny");
  expect(app.texts()).toContain("Approved: deny");
  await app.press("label:Allow");
  expect(app.texts()).toContain("Approved: allow");
});

test("the question card selects an option and submits it", async () => {
  const app = await open("./interaction_card.fixture.tsx", { platform: "linux" });
  // Nothing selected yet: the submit stays inert and reads "Continue", with
  // no answer reported.
  expect(app.texts()).toContain("Continue");
  expect(app.texts()).toContain("Answer: none");

  await app.press("label:Postgres");
  expect(app.texts()).toContain("Picked: Postgres");
  await app.press("label:Continue");
  expect(app.texts()).toContain('Answer: "Postgres"');
});

test("the question card skips and relabels the submit button when last", async () => {
  const app = await open("./interaction_card.fixture.tsx", { platform: "linux" });
  await app.press("label:Skip");
  expect(app.texts()).toContain("Skipped: yes");

  await app.press("label:Toggle last");
  await app.press("label:SQLite");
  await app.press("label:Submit");
  expect(app.texts()).toContain('Answer: "SQLite"');
});
