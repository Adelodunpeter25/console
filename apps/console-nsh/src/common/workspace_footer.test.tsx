// Tests for the workspace footer, beside `common/workspace_footer.tsx`. The
// runner opens `workspace_footer.fixture.tsx`, which moves its own state via
// scenario buttons and renders the callbacks it received on screen.
import { expect, open, test } from "nsh/test";

test("the footer shows its chips and the usage panel opens", async () => {
  const app = await open("./workspace_footer.fixture.tsx");
  // No project yet: the plain labels, and the branch chip disabled.
  expect(app.texts()).toContain("No project");
  expect(app.texts()).toContain("No branch");

  await app.press("label:Usage");
  // The usage meter's panel opens inside the footer.
  expect(app.texts()).toContain("Usage Limits");
});

test("the project menu lists projects, management rows and GitHub", async () => {
  const app = await open("./workspace_footer.fixture.tsx");
  await app.press("label:Load projects");

  // The menu carries the projects, the management rows and the GitHub section.
  await app.press("label:Project");
  expect(app.texts()).toContain("Repo One");
  expect(app.texts()).toContain("Repo Two");
  expect(app.texts()).toContain("New project…");
  expect(app.texts()).toContain("GitHub");

  await app.press("text:Repo Two");
  expect(app.texts()).toContain("Selected: p2");

  await app.press("label:Project");
  await app.press("text:New project…");
  expect(app.texts()).toContain("Action: new-project");
});

test("the branch chip moves through its states and opens the branch menu", async () => {
  const app = await open("./workspace_footer.fixture.tsx");
  await app.press("label:No git");
  expect(app.texts()).toContain("No Git repository");

  await app.press("label:Branch loading");
  expect(app.texts()).toContain("Loading…");

  await app.press("label:Branch switching");
  expect(app.texts()).toContain("Switching…");

  await app.press("label:Branches ready");
  expect(app.texts()).toContain("main");

  await app.press("label:Branch");
  expect(app.texts()).toContain("New worktree…");
  await app.press("text:feature/ui");
  expect(app.texts()).toContain("Selected: feature/ui");
});

test("a locked project chip keeps its label but no longer opens", async () => {
  const app = await open("./workspace_footer.fixture.tsx");
  await app.press("label:Load projects");
  await app.press("label:Select p2");
  await app.press("label:Lock");
  // The chip still names the project, but a locked chip is not pressable, so
  // its menu can no longer open.
  expect(app.texts()).toContain("Repo Two");
  let opened = true;
  await app
    .press("label:Project")
    .catch(() => {
      opened = false;
    });
  expect(opened).toBe(false);
  expect(app.texts()).not.toContain("New project…");
});

test("a running clone shows on the project chip", async () => {
  const app = await open("./workspace_footer.fixture.tsx");
  await app.press("label:Cloning");
  expect(app.texts()).toContain("Cloning acme/api…");
});
