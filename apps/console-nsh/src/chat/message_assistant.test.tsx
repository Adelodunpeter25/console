// Tests for the assistant message bubble, beside `message_assistant.tsx`:
// thinking disclosure, tool-call exclusion, throughput label, and the fork
// affordance's streaming gate.
import { expect, open, test } from "nsh/test";
import { formatTokensPerSecond } from "../utils/time";

test("formatTokensPerSecond hides absent and non-positive rates", () => {
  expect(formatTokensPerSecond(12.34)).toBe("12.3 tok/s");
  expect(formatTokensPerSecond(0)).toBeUndefined();
  expect(formatTokensPerSecond(null)).toBeUndefined();
});

test("the assistant bubble renders thinking and text, not inline tool calls", async () => {
  const app = await open("./message_assistant.fixture.tsx", { platform: "linux" });
  expect(app.texts()).toContain("THINKING");
  expect(app.texts()).toContain("Here is the answer, in full.");
  expect(app.texts()).toContain("12.3 tok/s");
  // Thinking is collapsed by default.
  expect(app.texts().some((t) => t.includes("Weigh the options"))).toBe(false);
  await app.press("label:thinking-header-part-0");
  expect(app.texts().some((t) => t.includes("Weigh the options"))).toBe(true);
});

test("fork is live only when the message is settled", async () => {
  const app = await open("./message_assistant.fixture.tsx", { platform: "linux" });
  await app.press("label:Fork from message assistant-7");
  expect(app.texts()).toContain("Forked: assistant-7");
  // While streaming, the fork affordance disappears.
  await app.press("label:Toggle streaming");
  expect(
    app.texts().some((t) => t.startsWith("Fork from message")),
  ).toBe(false);
});
