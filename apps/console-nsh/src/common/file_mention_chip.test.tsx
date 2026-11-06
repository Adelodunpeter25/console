// Tests for the file-mention chip, beside `file_mention_chip.tsx`.
import { expect, open, test } from "nsh/test";
import { mentionIconName } from "./file_mention_chip";

test("chips paint the right icon per mention kind", () => {
  expect(mentionIconName("src/main.rs")).toBe("fileType/Rust");
  expect(mentionIconName("chat:alpha")).toBe("ChatRoundLine");
  expect(mentionIconName("skill:review")).toBe("Sparkle");
});

test("the fixture renders all three mention kinds", async () => {
  const app = await open("./file_mention_chip.fixture.tsx", { platform: "linux" });
  for (const label of ["main.rs", "$alpha", "/review"]) {
    expect(app.texts()).toContain(label);
  }
});
