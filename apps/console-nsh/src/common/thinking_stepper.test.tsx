// Tests for the thinking stepper, beside `common/thinking_stepper.tsx`. The
// runner opens `thinking_stepper.fixture.tsx`, which cycles real levels.
import { expect, open, test } from "nsh/test";
import { thinkingStepLabel } from "./thinking_stepper";

test("the step label counts 1-based, or 0-based past a leading None", () => {
  expect(thinkingStepLabel("Low", ["Low", "Medium", "High"])).toBe("1/3");
  expect(thinkingStepLabel("High", ["Low", "Medium", "High"])).toBe("3/3");
  // With None first, off is step 0 and None itself is not counted.
  expect(thinkingStepLabel("None", ["None", "Low", "Medium"])).toBe("0/2");
  expect(thinkingStepLabel("Low", ["None", "Low", "Medium"])).toBe("1/2");
});

test("pressing the stepper cycles through the levels", async () => {
  const app = await open("./thinking_stepper.fixture.tsx");
  // The fixture's levels start at None: Low is step 1 of 3.
  expect(app.texts()).toContain("Level: Low");
  expect(app.texts()).toContain("1/3");

  await app.press("label:Thinking level");
  expect(app.texts()).toContain("Level: Medium");
  expect(app.texts()).toContain("2/3");

  await app.press("label:Thinking level");
  expect(app.texts()).toContain("Level: High");
  expect(app.texts()).toContain("3/3");

  // And it loops back to None, the off state.
  await app.press("label:Thinking level");
  expect(app.texts()).toContain("Level: None");
  expect(app.texts()).toContain("0/3");
});
