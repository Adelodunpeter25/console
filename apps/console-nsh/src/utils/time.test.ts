// Time-formatting tests for `utils/time.ts` (`console-ui/src/utils/time.rs`).
import { expect, test } from "nsh/test";
import { formatTimeAgo } from "./time";

test("formats time ago from seconds or milliseconds", () => {
  const now = 1_700_000_000; // seconds
  expect(formatTimeAgo(now - 30, now)).toBe("just now");
  expect(formatTimeAgo((now - 300) * 1000, now)).toBe("5m");
  expect(formatTimeAgo(now - 7200, now)).toBe("2h");
  expect(formatTimeAgo(now - 3 * 86_400, now)).toBe("3d");
});
