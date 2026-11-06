// Tests for the change chips, beside `chat/change_chips.tsx`. The summary
// helper is asserted directly; the chips run through
// `change_chips.fixture.tsx`.
import { expect, open, test } from "nsh/test";
import {
  changesSummaryFrom,
  baseName,
  fileTypeIconName,
  COLLAPSED_CHIPS,
} from "./change_chips";

test("the summary counts unique paths and sums churn", () => {
  const summary = changesSummaryFrom([
    { path: "a.ts", additions: 2, deletions: 0 },
    { path: "a.ts", additions: 1, deletions: 4 },
    { path: "b.ts", additions: 0, deletions: 1 },
  ]);
  expect(summary).toEqual({ files: 2, additions: 3, deletions: 5 });
});

test("helpers resolve basenames and file-type icons", () => {
  expect(baseName("src/chat/app.ts")).toBe("app.ts");
  expect(baseName("C:\\repo\\README.md")).toBe("README.md");
  expect(fileTypeIconName("src/main.rs")).toBe("fileType/Rust");
  expect(fileTypeIconName("unknown.zyx")).toBe("fileType/File");
});

test("a five-file turn folds to three chips plus the toggle", async () => {
  const app = await open("./change_chips.fixture.tsx", { platform: "linux" });
  expect(COLLAPSED_CHIPS).toBe(3);
  // Collapsed: the first three files show, the rest fold into "+2 more".
  expect(app.texts()).toContain("app.ts");
  expect(app.texts()).toContain("+12");
  expect(app.texts()).toContain("-3");
  expect(app.texts()).toContain("2 more");
  expect(app.texts().some((t) => t === "gone.ts")).toBe(false);
  expect(app.texts()).toContain("Expanded: no");
});

test("the fold toggle expands and re-collapses the row", async () => {
  const app = await open("./change_chips.fixture.tsx", { platform: "linux" });
  await app.press("label:Toggle change chips");
  expect(app.texts()).toContain("Show less");
  expect(app.texts()).toContain("gone.ts");
  expect(app.texts()).toContain("notes.txt");
  expect(app.texts()).toContain("Expanded: yes");
  await app.press("label:Toggle change chips");
  expect(app.texts()).toContain("2 more");
  expect(app.texts()).toContain("Expanded: no");
});

test("a chip without a diff opens the file directly", async () => {
  const app = await open("./change_chips.fixture.tsx", { platform: "linux" });
  // notes.txt sits past the collapsed chips, so expand first; it has no diff
  // text and is not binary: no popover, press opens.
  await app.press("label:Toggle change chips");
  await app.press("label:Show diff for notes.txt");
  expect(app.texts()).toContain("Action: open:notes.txt");
});
