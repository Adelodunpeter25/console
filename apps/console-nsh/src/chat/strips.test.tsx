// Tests for the small chat strips: the working indicator's elapsed label and
// the notice banner, ported from console-ui's `chat/working_indicator.rs`
// and `common/notice_banner.rs`.
import { expect, open, test } from "nsh/test";
import { formatWorkingElapsed } from "../utils/time";

test("the working label reads 37s, 1m 5s, 1h 2m", () => {
  expect(formatWorkingElapsed(0)).toBe("0s");
  expect(formatWorkingElapsed(37)).toBe("37s");
  expect(formatWorkingElapsed(65)).toBe("1m 5s");
  expect(formatWorkingElapsed(3_600)).toBe("1h");
  expect(formatWorkingElapsed(3_720)).toBe("1h 2m");
});

test("the working indicator renders and ticks", async () => {
  const app = await open("./working_indicator.fixture.tsx", { platform: "linux" });
  const label = (texts: string[]) =>
    Number(texts.find((t) => /^Working for \d+s$/.test(t))?.match(/\d+/)?.[0]);
  const before = label(app.texts());
  // The 1-second label ticks without any test step driving it.
  await app.wait(2100);
  expect(label(app.texts())).toBeGreaterThanOrEqual(before + 2);
});

test("the notice banner shows its message", async () => {
  const app = await open("./notice_banner.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("Context compacted — earlier messages summarized");
});
